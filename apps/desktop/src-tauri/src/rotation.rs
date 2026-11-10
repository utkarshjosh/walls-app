//! Rotation: what is on the desktop, what comes next, and when.
//!
//! One background thread (the "worker") owns all of it: the plan the UI last
//! sent, the engine state, the live windows and the timer. Commands, the tray
//! and display notifications only post messages to it. That gives every change
//! a single order and means no lock is ever held while talking to AppKit: the
//! worker hops to the main thread for those calls, and the main thread never
//! waits for the worker.
//!
//! The state is saved after every change and restored at launch, so rotation
//! carries on across restarts and with the window closed.

use std::fs;
use std::path::{Path, PathBuf};
use std::sync::mpsc::{self, Receiver, RecvTimeoutError, Sender, SyncSender};
use std::sync::{Arc, Mutex, PoisonError};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter};

use crate::displays;
use crate::engine::{self, EngineState, Rng};
use crate::library::Library;
use crate::live::{LiveTarget, LiveWindows, Viewport};
use crate::model::{
    ApplyRequest, CurrentEntry, DisplayInfo, DisplayMode, PlayItem, RotationCommand, RotationPlan, RotationStatus,
};
use crate::tray::Tray;
use crate::{power, schedule, wallpaper, MAIN_WINDOW};

/// The timer never sleeps longer than this, so a wake from sleep or a changed
/// clock is noticed promptly even though no boundary was reached "on time".
const LONGEST_SLEEP: Duration = Duration::from_secs(30);
const POWER_POLL: Duration = Duration::from_secs(15);
/// Display notifications arrive in bursts while macOS rearranges things.
const DISPLAY_SETTLE: Duration = Duration::from_millis(600);
const SHORTEST_SLEEP: Duration = Duration::from_millis(20);

const NOTHING_USABLE: &str = "None of the wallpapers in this rotation are in the library any more.";
const NOT_IN_LIBRARY: &str = "That file is not in the library.";
const NO_DISPLAY: &str = "No display is connected.";

// ---------------------------------------------------------------------------
// The handle the rest of the app holds
// ---------------------------------------------------------------------------

type Reply = SyncSender<Result<RotationStatus, String>>;

pub enum Message {
    Sync(RotationPlan, Reply),
    Command(RotationCommand, Option<Reply>),
    Apply(ApplyRequest, Reply),
    DisplaysChanged,
}

#[derive(Clone)]
pub struct Rotation {
    sender: Sender<Message>,
    status: Arc<Mutex<RotationStatus>>,
}

impl Rotation {
    /// The handle plus the inbox to give to `spawn`. Split in two because the
    /// tray needs the handle before the worker (which owns the tray) exists.
    pub fn channel() -> (Rotation, Receiver<Message>) {
        let (sender, inbox) = mpsc::channel();
        (Rotation { sender, status: Arc::default() }, inbox)
    }

    /// The latest status, without waiting for the worker.
    pub fn status(&self) -> RotationStatus {
        self.status.lock().unwrap_or_else(PoisonError::into_inner).clone()
    }

    // The three calls below block until the worker has finished, so they must
    // only be made from background threads (commands), never the main thread.

    pub fn sync(&self, plan: RotationPlan) -> Result<RotationStatus, String> {
        self.ask(|reply| Message::Sync(plan, reply))
    }

    pub fn command(&self, command: RotationCommand) -> Result<RotationStatus, String> {
        self.ask(|reply| Message::Command(command, Some(reply)))
    }

    pub fn apply(&self, request: ApplyRequest) -> Result<RotationStatus, String> {
        self.ask(|reply| Message::Apply(request, reply))
    }

    /// Fire and forget, for the main thread (tray menu).
    pub fn post(&self, command: RotationCommand) {
        let _ = self.sender.send(Message::Command(command, None));
    }

    /// Fire and forget, for the main thread (display notifications).
    pub fn displays_changed(&self) {
        let _ = self.sender.send(Message::DisplaysChanged);
    }

    fn ask(&self, message: impl FnOnce(Reply) -> Message) -> Result<RotationStatus, String> {
        let stopped = || "Still is shutting down.".to_string();
        let (reply, answer) = mpsc::sync_channel(1);
        self.sender.send(message(reply)).map_err(|_| stopped())?;
        answer.recv().map_err(|_| stopped())?
    }
}

// ---------------------------------------------------------------------------
// Saved state and the pure rules around it
// ---------------------------------------------------------------------------

/// How the current frame maps onto displays.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "kind", content = "displays", rename_all = "camelCase")]
pub enum Layout {
    /// Entry 0 on every display.
    #[default]
    Same,
    /// Entry 0 stretched across all displays as one canvas.
    Span,
    /// Entry `i` on the display with id `displays[i]`. Ids rather than
    /// positions, so each display keeps its wallpaper when they are rearranged.
    PerDisplay(Vec<String>),
}

#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct Saved {
    pub plan: Option<RotationPlan>,
    pub paused: bool,
    pub engine: EngineState,
    pub layout: Layout,
    pub next_change_at: Option<i64>,
    /// Wallpapers shown by hand that are not part of the plan, kept so that
    /// "previous" and re-applying after a display change can still find them.
    pub extras: Vec<PlayItem>,
}

/// One display and what should be on it.
#[derive(Debug, Clone, PartialEq)]
pub struct Target {
    pub display: DisplayInfo,
    pub item: PlayItem,
    pub viewport: Viewport,
    /// True when this display shows its part of a picture spanned over all displays.
    pub spanned: bool,
}

pub fn status_of(saved: &Saved) -> RotationStatus {
    let current = match &saved.layout {
        Layout::PerDisplay(display_ids) => display_ids
            .iter()
            .zip(&saved.engine.current)
            .map(|(display_id, item_id)| CurrentEntry { display_id: Some(display_id.clone()), item_id: item_id.clone() })
            .collect(),
        Layout::Same | Layout::Span => saved
            .engine
            .current
            .first()
            .map(|item_id| CurrentEntry { display_id: None, item_id: item_id.clone() })
            .into_iter()
            .collect(),
    };
    RotationStatus {
        paused: saved.paused,
        current,
        next_change_at: saved.next_change_at,
        can_go_previous: !saved.engine.history.is_empty(),
    }
}

/// How many wallpapers a frame holds, and how it is laid out, for a display mode.
pub fn shape_for(mode: DisplayMode, displays: &[DisplayInfo]) -> (usize, Layout) {
    match mode {
        DisplayMode::Same => (1, Layout::Same),
        DisplayMode::Span => (1, Layout::Span),
        DisplayMode::Each => {
            let ids: Vec<String> = displays::reading_order(displays).into_iter().map(|display| display.id).collect();
            (ids.len().max(1), Layout::PerDisplay(ids))
        }
    }
}

/// Which item goes on which display. `items` lines up with the frame.
pub fn plan_targets(layout: &Layout, items: &[PlayItem], displays: &[DisplayInfo]) -> Vec<Target> {
    let own_screen = |display: &DisplayInfo| Viewport {
        canvas_width: display.width,
        canvas_height: display.height,
        x: 0.0,
        y: 0.0,
    };
    match layout {
        Layout::PerDisplay(display_ids) => display_ids
            .iter()
            .zip(items)
            .filter_map(|(display_id, item)| {
                let display = displays.iter().find(|display| display.id == *display_id)?;
                Some(Target { display: display.clone(), item: item.clone(), viewport: own_screen(display), spanned: false })
            })
            .collect(),
        Layout::Same | Layout::Span => {
            let Some(item) = items.first() else {
                return Vec::new();
            };
            // With one display there is nothing to span: behave like "same".
            let canvas = match layout {
                Layout::Span if displays.len() > 1 => wallpaper::canvas_bounds(displays),
                _ => None,
            };
            displays
                .iter()
                .map(|display| {
                    let viewport = match &canvas {
                        Some(canvas) => {
                            let (x, y) = wallpaper::offset_in_canvas(canvas, display);
                            Viewport { canvas_width: canvas.width, canvas_height: canvas.height, x, y }
                        }
                        None => own_screen(display),
                    };
                    Target { display: display.clone(), item: item.clone(), viewport, spanned: canvas.is_some() }
                })
                .collect()
        }
    }
}

/// What each display in `ordered` shows according to the frame, or `None`
/// where the frame says nothing about that display.
pub fn per_display_view(current: &[String], layout: &Layout, ordered: &[String]) -> Vec<Option<String>> {
    ordered
        .iter()
        .map(|display_id| match layout {
            Layout::PerDisplay(display_ids) => display_ids
                .iter()
                .position(|known| known == display_id)
                .and_then(|slot| current.get(slot).cloned()),
            Layout::Same | Layout::Span => current.first().cloned(),
        })
        .collect()
}

/// Fill the unknown entries of a per-display view: first from `drawn`, then by
/// repeating `fallback`. A display that still has nothing is left out, so the
/// returned display ids and frame always line up.
pub fn fill_view(
    ordered: &[String],
    view: Vec<Option<String>>,
    drawn: &[String],
    fallback: &[String],
) -> (Vec<String>, Vec<String>) {
    let mut drawn = drawn.iter();
    let mut repeats = 0usize;
    let mut display_ids = Vec::with_capacity(ordered.len());
    let mut frame = Vec::with_capacity(ordered.len());
    for (display_id, known) in ordered.iter().zip(view) {
        let item_id = known.or_else(|| drawn.next().cloned()).or_else(|| {
            let repeated = fallback.get(repeats % fallback.len().max(1)).cloned();
            repeats += 1;
            repeated
        });
        if let Some(item_id) = item_id {
            display_ids.push(display_id.clone());
            frame.push(item_id);
        }
    }
    (display_ids, frame)
}

/// Fit a frame from history onto today's display mode and displays.
pub fn fit_frame(frame: &[String], mode: DisplayMode, displays: &[DisplayInfo]) -> (Vec<String>, Layout) {
    let (count, layout) = shape_for(mode, displays);
    if frame.is_empty() {
        return (Vec::new(), layout);
    }
    let fitted = (0..count).map(|slot| frame[slot % frame.len()].clone()).collect();
    (fitted, layout)
}

/// Forget wallpapers that are gone, keeping a per-display layout lined up with
/// the entries that survive.
pub fn reconcile_saved(saved: &mut Saved, alive: &[String]) {
    if let Layout::PerDisplay(display_ids) = &saved.layout {
        let kept = display_ids
            .iter()
            .zip(&saved.engine.current)
            .filter(|(_, item_id)| alive.contains(item_id))
            .map(|(display_id, _)| display_id.clone())
            .collect();
        saved.layout = Layout::PerDisplay(kept);
    }
    saved.engine = engine::reconcile(&saved.engine, alive);
}

fn now_ms() -> i64 {
    SystemTime::now().duration_since(UNIX_EPOCH).map_or(0, |since| since.as_millis() as i64)
}

fn load(file: &Path) -> Saved {
    match fs::read_to_string(file) {
        Ok(text) => serde_json::from_str(&text).unwrap_or_else(|error| {
            // Start clean rather than refuse to launch; the UI re-sends the plan.
            eprintln!("still: ignoring unreadable rotation state ({error})");
            Saved::default()
        }),
        Err(_) => Saved::default(),
    }
}

// ---------------------------------------------------------------------------
// The worker
// ---------------------------------------------------------------------------

pub struct WorkerSetup {
    pub app: AppHandle,
    pub library: Library,
    pub state_file: PathBuf,
    pub span_cache: PathBuf,
    pub tray: Tray,
}

pub fn spawn(rotation: &Rotation, inbox: Receiver<Message>, setup: WorkerSetup) -> std::io::Result<()> {
    let seed = now_ms() as u64 ^ (u64::from(std::process::id()) << 32);
    let worker = Worker {
        app: setup.app,
        library: setup.library,
        state_file: setup.state_file,
        span_cache: setup.span_cache,
        tray: setup.tray,
        shared: rotation.status.clone(),
        saved: Saved::default(),
        last_written: String::new(),
        displays: Vec::new(),
        live: LiveWindows::default(),
        rng: Rng::from_seed(seed),
        arrangement_changed: false,
        displays_dirty_since: None,
        last_display_check: Instant::now(),
        last_power_check: Instant::now(),
    };
    std::thread::Builder::new().name("still-rotation".into()).spawn(move || worker.run(inbox)).map(|_| ())
}

struct Worker {
    app: AppHandle,
    library: Library,
    state_file: PathBuf,
    span_cache: PathBuf,
    tray: Tray,
    shared: Arc<Mutex<RotationStatus>>,
    saved: Saved,
    last_written: String,
    displays: Vec<DisplayInfo>,
    live: LiveWindows,
    rng: Rng,
    /// The display list differs from the one the current frame was last laid
    /// out for; `check_displays` re-applies and clears it.
    arrangement_changed: bool,
    displays_dirty_since: Option<Instant>,
    last_display_check: Instant,
    last_power_check: Instant,
}

/// Why a frame could not be shown. When one wallpaper is to blame, its id
/// lets automatic rotation skip it and try another.
struct ShowError {
    message: String,
    item_id: Option<String>,
}

impl ShowError {
    fn general(message: impl Into<String>) -> Self {
        ShowError { message: message.into(), item_id: None }
    }

    fn about(item_id: &str, message: impl Into<String>) -> Self {
        ShowError { message: message.into(), item_id: Some(item_id.to_string()) }
    }
}

#[derive(Clone, Copy, PartialEq)]
enum Part {
    /// Desktop pictures and live windows.
    Everything,
    /// Only bring live windows back (after a restart); pictures are untouched.
    LiveOnly,
}

impl Worker {
    fn run(mut self, inbox: Receiver<Message>) {
        self.launch();
        loop {
            match inbox.recv_timeout(self.next_wait()) {
                Ok(message) => self.handle(message),
                Err(RecvTimeoutError::Timeout) => {}
                Err(RecvTimeoutError::Disconnected) => break,
            }
            self.housekeeping();
            self.commit();
        }
    }

    fn launch(&mut self) {
        self.saved = load(&self.state_file);
        self.last_written = serde_json::to_string_pretty(&self.saved).unwrap_or_default();
        // The first hop to the main thread also waits for the event loop to start.
        self.refresh_displays();
        // Learning the displays for the first time is not a rearrangement:
        // nothing may be re-applied just because the app was launched.
        self.arrangement_changed = false;

        // A live wallpaper does not survive a restart by itself: the desktop
        // keeps only its poster. Bring the windows back; no picture is changed.
        let frame = self.saved.engine.current.clone();
        let layout = self.saved.layout.clone();
        let has_live = frame.iter().any(|id| self.item(id).is_some_and(|item| item.kind.is_live()));
        if has_live {
            if let Err(error) = self.show_frame(&frame, &layout, None, Part::LiveOnly) {
                eprintln!("still: could not restore the live wallpaper: {}", error.message);
            }
        }

        self.tick(true);
        self.commit();
        // The menu starts from defaults; make it match even if nothing "changed".
        self.tray.sync(&status_of(&self.saved));
    }

    fn handle(&mut self, message: Message) {
        match message {
            Message::Sync(plan, reply) => {
                let result = self.sync(plan);
                self.finish(result, Some(reply));
            }
            Message::Command(command, reply) => {
                let result = self.command(command);
                self.finish(result, reply);
            }
            Message::Apply(request, reply) => {
                let result = self.apply(request);
                self.finish(result, Some(reply));
            }
            Message::DisplaysChanged => self.displays_dirty_since = Some(Instant::now()),
        }
    }

    fn finish(&mut self, result: Result<(), String>, reply: Option<Reply>) {
        // Save and announce first, so the status in the reply is already current.
        self.commit();
        match (reply, result) {
            (Some(reply), result) => {
                let _ = reply.send(result.map(|()| status_of(&self.saved)));
            }
            (None, Err(error)) => eprintln!("still: {error}"),
            (None, Ok(())) => {}
        }
    }

    /// How long to wait for a message before looking at the clock again.
    fn next_wait(&self) -> Duration {
        let mut wait = LONGEST_SLEEP;
        if let Some(due) = self.saved.next_change_at {
            let until_due = (due - now_ms()).max(0) as u64;
            wait = wait.min(Duration::from_millis(until_due));
        }
        if let Some(since) = self.displays_dirty_since {
            wait = wait.min(DISPLAY_SETTLE.saturating_sub(since.elapsed()));
        }
        if self.wants_power_checks() {
            wait = wait.min(POWER_POLL.saturating_sub(self.last_power_check.elapsed()));
        }
        wait.max(SHORTEST_SLEEP)
    }

    fn housekeeping(&mut self) {
        let settled = self.displays_dirty_since.is_some_and(|since| since.elapsed() >= DISPLAY_SETTLE);
        // The periodic re-check is a safety net in case a notification was missed.
        if settled || self.arrangement_changed || self.last_display_check.elapsed() >= LONGEST_SLEEP {
            if settled {
                self.displays_dirty_since = None;
            }
            self.check_displays();
        }
        if self.wants_power_checks() && self.last_power_check.elapsed() >= POWER_POLL {
            self.update_live_pause();
        }
        self.tick(false);
    }

    /// Save the state if it changed, and tell the UI and the tray if the status changed.
    fn commit(&mut self) {
        if let Ok(json) = serde_json::to_string_pretty(&self.saved) {
            if json != self.last_written {
                if let Err(error) = write_atomically(&self.state_file, &json) {
                    eprintln!("still: could not save rotation state: {error}");
                }
                self.last_written = json;
            }
        }
        let status = status_of(&self.saved);
        let changed = {
            let mut shared = self.shared.lock().unwrap_or_else(PoisonError::into_inner);
            let changed = *shared != status;
            *shared = status.clone();
            changed
        };
        if changed {
            let _ = self.app.emit_to(MAIN_WINDOW, "rotation-changed", &status);
            self.tray.sync(&status);
        }
    }

    // --- the timer ---------------------------------------------------------

    /// Compare the wall clock with the next due time. At most one change
    /// happens however many boundaries were missed (sleep, clock change), and
    /// the next boundary is always worked out from "now".
    fn tick(&mut self, at_launch: bool) {
        let fresh = self.fresh_boundary();
        match self.saved.next_change_at {
            Some(due) if fresh.is_some() && now_ms() >= due => {
                // Never surprise someone at launch who has not set a wallpaper with Still yet.
                let never_shown = self.saved.engine.current.is_empty();
                if !(at_launch && never_shown) {
                    if let Err(error) = self.advance() {
                        eprintln!("still: scheduled change skipped: {error}");
                    }
                }
                // Recomputed after the change, which can take a while.
                self.saved.next_change_at = self.fresh_boundary();
            }
            // Not due. Re-deriving the boundary keeps it right when the time
            // zone or the clock changes; normally it is the same value.
            _ => self.saved.next_change_at = fresh,
        }
    }

    /// The first boundary after now, or `None` while paused or with nothing to rotate.
    fn fresh_boundary(&self) -> Option<i64> {
        let plan = self.saved.plan.as_ref()?;
        schedule::next_change(&plan.schedule, self.saved.paused, plan.items.len(), &chrono::Local::now())
            .map(|next| next.timestamp_millis())
    }

    // --- messages ----------------------------------------------------------

    fn sync(&mut self, plan: RotationPlan) -> Result<(), String> {
        let previous = self.saved.plan.replace(plan.clone());
        self.saved.paused = plan.paused;

        // Hand-shown wallpapers stay known until their file goes or the plan covers them.
        let library = &self.library;
        self.saved
            .extras
            .retain(|extra| !plan.items.iter().any(|item| item.id == extra.id) && library.holds(&extra.path));
        let alive: Vec<String> = plan.items.iter().chain(&self.saved.extras).map(|item| item.id.clone()).collect();
        reconcile_saved(&mut self.saved, &alive);
        self.forget_unused_extras();

        // Keep a boundary that is already set unless the schedule itself
        // changed: recomputing "from now" on every sync could step over a
        // change that is due this very moment.
        let schedule_changed = previous.as_ref().is_none_or(|old| old.schedule != plan.schedule);
        self.saved.next_change_at = match (self.fresh_boundary(), self.saved.next_change_at) {
            (None, _) => None,
            (Some(_), Some(kept)) if !schedule_changed => Some(kept),
            (fresh, _) => fresh,
        };

        // Syncing never changes the wallpaper, with one exception: a new
        // display mode re-lays-out what is already showing.
        let mode_changed = previous.as_ref().is_some_and(|old| old.display_mode != plan.display_mode);
        if mode_changed && !self.saved.engine.current.is_empty() {
            if let Err(error) = self.reshape(plan.display_mode) {
                eprintln!("still: could not re-apply the wallpaper in the new display mode: {error}");
            }
        }
        self.update_live_pause();
        Ok(())
    }

    fn command(&mut self, command: RotationCommand) -> Result<(), String> {
        match command {
            RotationCommand::Next => self.advance().map(|_| ()),
            RotationCommand::Previous => self.previous().map(|_| ()),
            RotationCommand::Pause => {
                self.set_paused(true);
                Ok(())
            }
            RotationCommand::Resume => {
                self.set_paused(false);
                let has_items = self.saved.plan.as_ref().is_some_and(|plan| !plan.items.is_empty());
                if has_items && self.saved.engine.current.is_empty() {
                    self.advance()?;
                }
                Ok(())
            }
        }
    }

    fn set_paused(&mut self, paused: bool) {
        self.saved.paused = paused;
        if let Some(plan) = &mut self.saved.plan {
            plan.paused = paused;
        }
        self.saved.next_change_at = self.fresh_boundary();
    }

    fn apply(&mut self, request: ApplyRequest) -> Result<(), String> {
        let Some(first) = request.items.first().cloned() else {
            return Err("There is nothing to show.".to_string());
        };
        self.refresh_displays();
        if self.displays.is_empty() {
            return Err(NO_DISPLAY.to_string());
        }
        let ordered = displays::reading_order(&self.displays);
        let plan_mode = self.saved.plan.as_ref().map(|plan| plan.display_mode);

        // "Only on this display": meaningful when each display has its own
        // wallpaper and there is more than one. Otherwise (or for a display id
        // we do not know) it is an ordinary apply to every display.
        let single_slot = match (&request.mode, &request.display_id) {
            (DisplayMode::Same, Some(display_id)) if plan_mode == Some(DisplayMode::Each) && ordered.len() > 1 => {
                ordered.iter().position(|display| display.id == *display_id)
            }
            _ => None,
        };

        let (next, layout, targets) = if let Some(slot) = single_slot {
            let (staged, layout) = self.per_display_state(true, std::slice::from_ref(&first.id));
            let next = engine::show_in_slot(&staged, slot, &first.id);
            // Only this screen is touched; the others keep what they show.
            let only = Layout::PerDisplay(vec![ordered[slot].id.clone()]);
            let targets = plan_targets(&only, std::slice::from_ref(&first), &self.displays);
            (next, layout, targets)
        } else {
            let (count, layout) = shape_for(request.mode, &self.displays);
            let items: Vec<PlayItem> =
                (0..count).map(|slot| request.items[slot % request.items.len()].clone()).collect();
            let frame: Vec<String> = items.iter().map(|item| item.id.clone()).collect();
            let targets = plan_targets(&layout, &items, &self.displays);
            (engine::show(&self.saved.engine, &frame), layout, targets)
        };

        self.show_targets(targets, Part::Everything).map_err(|error| error.message)?;
        self.saved.engine = next;
        self.saved.layout = layout;
        self.remember_extras(&request.items);
        Ok(())
    }

    // --- changing what is shown ----------------------------------------------

    /// Automatic or "next": draw the next frame and show it. A wallpaper whose
    /// file is missing or unusable is skipped in favour of another, each at
    /// most once, so one bad file never stops rotation or blanks the desktop.
    /// `Ok(false)` means there was nothing to rotate.
    fn advance(&mut self) -> Result<bool, String> {
        let Some(plan) = self.saved.plan.clone() else {
            return Ok(false);
        };
        if plan.items.is_empty() {
            return Ok(false);
        }
        self.refresh_displays();
        if self.displays.is_empty() {
            return Err(NO_DISPLAY.to_string());
        }
        let (count, layout) = shape_for(plan.display_mode, &self.displays);
        let mut usable = self.usable_ids(&plan);
        let mut last_error = NOTHING_USABLE.to_string();
        while !usable.is_empty() {
            let next = {
                let rng = &mut self.rng;
                engine::advance(&self.saved.engine, &usable, plan.shuffle, count, &mut || rng.next_f64())
            };
            match self.show_frame(&next.current, &layout, None, Part::Everything) {
                Ok(()) => {
                    self.saved.engine = next;
                    self.saved.layout = layout;
                    return Ok(true);
                }
                Err(ShowError { message, item_id: Some(bad) }) => {
                    usable.retain(|id| *id != bad);
                    last_error = message;
                }
                Err(error) => return Err(error.message),
            }
        }
        Err(last_error)
    }

    /// Step back through what was really shown. History entries that can no
    /// longer be shown (file deleted since) are dropped and the one before is tried.
    fn previous(&mut self) -> Result<bool, String> {
        self.refresh_displays();
        let mode = self.saved.plan.as_ref().map_or(DisplayMode::Same, |plan| plan.display_mode);
        let mut last_error = None;
        while let Some(stepped) = engine::back(&self.saved.engine) {
            let (frame, layout) = fit_frame(&stepped.current, mode, &self.displays);
            match self.show_frame(&frame, &layout, None, Part::Everything) {
                Ok(()) => {
                    self.saved.engine = EngineState { current: frame, ..stepped };
                    self.saved.layout = layout;
                    return Ok(true);
                }
                Err(error) if error.item_id.is_some() => {
                    self.saved.engine.history.pop();
                    last_error = Some(error.message);
                }
                Err(error) => return Err(error.message),
            }
        }
        last_error.map_or(Ok(false), Err)
    }

    /// The display mode changed: lay out what is showing the new way.
    fn reshape(&mut self, mode: DisplayMode) -> Result<(), String> {
        self.refresh_displays();
        let (count, layout) = shape_for(mode, &self.displays);
        // For "each" the frame is topped up to one wallpaper per display by
        // drawing more; for the others it shrinks to its first entry.
        let ids = self.saved.plan.clone().map(|plan| (self.usable_ids(&plan), plan.shuffle));
        let (ids, shuffle) = ids.unwrap_or_default();
        let next = {
            let rng = &mut self.rng;
            engine::refit(&self.saved.engine, &ids, shuffle, count, &mut || rng.next_f64())
        };
        self.show_frame(&next.current, &layout, None, Part::Everything).map_err(|error| error.message)?;
        self.saved.engine = next;
        self.saved.layout = layout;
        Ok(())
    }

    /// The current frame as one entry per connected display (reading order).
    /// Displays the frame says nothing about are filled by drawing from the
    /// plan (`draw`), else by repeating what is showing, else from `fallback`.
    /// Not a change in itself: no history entry.
    fn per_display_state(&mut self, draw: bool, fallback: &[String]) -> (EngineState, Layout) {
        let ordered: Vec<String> =
            displays::reading_order(&self.displays).into_iter().map(|display| display.id).collect();
        let view = per_display_view(&self.saved.engine.current, &self.saved.layout, &ordered);
        let known: Vec<String> = view.iter().flatten().cloned().collect();
        let missing = view.len() - known.len();

        let mut staged = self.saved.engine.clone();
        let mut drawn = Vec::new();
        if draw && missing > 0 {
            if let Some(plan) = self.saved.plan.clone() {
                let ids = self.usable_ids(&plan);
                let seed = EngineState { current: known.clone(), ..staged.clone() };
                let rng = &mut self.rng;
                let fitted = engine::refit(&seed, &ids, plan.shuffle, known.len() + missing, &mut || rng.next_f64());
                drawn = fitted.current.iter().skip(known.len()).cloned().collect();
                staged.bag = fitted.bag;
            }
        }
        let repeat_from =
            if self.saved.engine.current.is_empty() { fallback } else { self.saved.engine.current.as_slice() };
        let (display_ids, frame) = fill_view(&ordered, view, &drawn, repeat_from);
        staged.current = frame;
        (staged, Layout::PerDisplay(display_ids))
    }

    fn show_frame(
        &mut self,
        frame: &[String],
        layout: &Layout,
        only_display: Option<&str>,
        part: Part,
    ) -> Result<(), ShowError> {
        let mut items = Vec::with_capacity(frame.len());
        for id in frame {
            match self.item(id) {
                Some(item) => items.push(item),
                None => return Err(ShowError::about(id, "That wallpaper is no longer available.")),
            }
        }
        let mut targets = plan_targets(layout, &items, &self.displays);
        if let Some(display_id) = only_display {
            targets.retain(|target| target.display.id == display_id);
        }
        self.show_targets(targets, part)
    }

    /// The one place wallpapers actually change.
    fn show_targets(&mut self, targets: Vec<Target>, part: Part) -> Result<(), ShowError> {
        if targets.is_empty() {
            return Err(ShowError::general(NO_DISPLAY));
        }
        for target in &targets {
            if !self.library.holds(&target.item.path) {
                return Err(ShowError::about(&target.item.id, NOT_IN_LIBRARY));
            }
        }

        if part == Part::Everything {
            self.set_stills(&targets)?;
        }

        // Stills first, live windows second: when a display goes from live to
        // still, its new picture is already in place before the window closes.
        let wanted: Vec<LiveTarget> = targets
            .iter()
            .filter(|target| target.item.kind.is_live())
            .map(|target| LiveTarget {
                display: target.display.clone(),
                item: target.item.clone(),
                viewport: target.viewport,
            })
            .collect();
        let scope: Vec<String> = targets.iter().map(|target| target.display.id.clone()).collect();
        let connected: Vec<String> = self.displays.iter().map(|display| display.id.clone()).collect();
        let paused = self.live_should_pause();
        if let Err(error) = self.live.sync(&self.app, &wanted, &scope, &connected, paused) {
            // The poster is already on the desktop, so the change itself stands.
            eprintln!("still: live wallpaper problem: {error}");
        }
        self.last_power_check = Instant::now();
        Ok(())
    }

    /// Set the ordinary desktop picture for each target: the image itself, or
    /// a live wallpaper's poster so the lock screen and Mission Control match.
    fn set_stills(&self, targets: &[Target]) -> Result<(), ShowError> {
        let library = &self.library;
        let still_for = |item: &PlayItem| -> Option<PathBuf> {
            if item.kind.is_live() {
                // A live wallpaper without a usable poster leaves the picture underneath as it is.
                item.poster_path.as_deref().filter(|poster| library.holds(poster)).map(PathBuf::from)
            } else {
                Some(PathBuf::from(&item.path))
            }
        };
        // If macOS refuses and one wallpaper fills the whole frame, it is the one to skip.
        let sole_item = targets
            .first()
            .filter(|first| targets.iter().all(|target| target.item.id == first.item.id))
            .map(|first| first.item.id.clone());

        let spanned: Vec<&Target> = targets.iter().filter(|target| target.spanned).collect();
        let mut pictures: Vec<(String, PathBuf)> = Vec::new();
        let mut span_pieces: Option<Vec<PathBuf>> = None;
        if let Some(first) = spanned.first() {
            if let Some(source) = still_for(&first.item) {
                let span_displays: Vec<DisplayInfo> = spanned.iter().map(|target| target.display.clone()).collect();
                let pieces = wallpaper::render_span(&source, &first.item.id, &span_displays, &self.span_cache)
                    .map_err(|message| ShowError::about(&first.item.id, message))?;
                span_pieces = Some(pieces.iter().map(|(_, path)| path.clone()).collect());
                pictures.extend(pieces);
            }
        }
        for target in targets.iter().filter(|target| !target.spanned) {
            if let Some(path) = still_for(&target.item) {
                pictures.push((target.display.id.clone(), path));
            }
        }

        wallpaper::set_pictures(&self.app, pictures)
            .map_err(|message| ShowError { message, item_id: sole_item })?;
        if let Some(keep) = span_pieces {
            // Only now are the previous pieces off every desktop.
            wallpaper::prune_span_cache(&self.span_cache, &keep);
        }
        Ok(())
    }

    // --- displays and power --------------------------------------------------

    fn refresh_displays(&mut self) {
        self.last_display_check = Instant::now();
        match displays::fetch(&self.app) {
            Ok(fresh) if fresh != self.displays => {
                self.displays = fresh;
                self.arrangement_changed = true;
                let _ = self.app.emit_to(MAIN_WINDOW, "displays-changed", &self.displays);
            }
            Ok(_) => {}
            Err(error) => eprintln!("still: could not read the displays: {error}"),
        }
    }

    /// Displays may have been added, removed or rearranged: tell the UI and
    /// put the current frame onto the new arrangement.
    fn check_displays(&mut self) {
        self.refresh_displays();
        // With every display gone (lid closed, nothing attached) there is
        // nothing to lay out; their return counts as a new change.
        if !std::mem::take(&mut self.arrangement_changed) || self.displays.is_empty() {
            return;
        }
        if self.saved.engine.current.is_empty() {
            // Nothing of ours to re-apply; just drop windows for vanished displays.
            let connected: Vec<String> = self.displays.iter().map(|display| display.id.clone()).collect();
            let _ = self.live.sync(&self.app, &[], &[], &connected, false);
            return;
        }
        let (next, layout) = if matches!(self.saved.layout, Layout::PerDisplay(_)) {
            // Each display keeps its wallpaper; a newly connected one gets a
            // fresh pick when the plan gives every display its own.
            let each = self.saved.plan.as_ref().is_some_and(|plan| plan.display_mode == DisplayMode::Each);
            self.per_display_state(each, &[])
        } else {
            (self.saved.engine.clone(), self.saved.layout.clone())
        };
        match self.show_frame(&next.current, &layout, None, Part::Everything) {
            Ok(()) => {
                self.saved.engine = next;
                self.saved.layout = layout;
            }
            Err(error) => eprintln!("still: could not re-apply after a display change: {}", error.message),
        }
    }

    fn wants_power_checks(&self) -> bool {
        !self.live.is_empty() && self.saved.plan.as_ref().is_some_and(|plan| plan.pause_live_on_battery)
    }

    fn live_should_pause(&self) -> bool {
        self.saved.plan.as_ref().is_some_and(|plan| plan.pause_live_on_battery) && power::on_battery()
    }

    fn update_live_pause(&mut self) {
        self.last_power_check = Instant::now();
        if self.live.is_empty() {
            return;
        }
        let paused = self.live_should_pause();
        self.live.set_paused(&self.app, paused);
    }

    // --- items ---------------------------------------------------------------

    fn item(&self, id: &str) -> Option<PlayItem> {
        let in_plan = self.saved.plan.as_ref().and_then(|plan| plan.items.iter().find(|item| item.id == id));
        in_plan.or_else(|| self.saved.extras.iter().find(|item| item.id == id)).cloned()
    }

    /// Plan wallpapers whose file is present right now, in plan order.
    fn usable_ids(&self, plan: &RotationPlan) -> Vec<String> {
        plan.items.iter().filter(|item| self.library.holds(&item.path)).map(|item| item.id.clone()).collect()
    }

    fn remember_extras(&mut self, items: &[PlayItem]) {
        for item in items {
            let in_plan = self.saved.plan.as_ref().is_some_and(|plan| plan.items.iter().any(|known| known.id == item.id));
            if in_plan {
                continue;
            }
            self.saved.extras.retain(|extra| extra.id != item.id);
            self.saved.extras.push(item.clone());
        }
        self.forget_unused_extras();
    }

    fn forget_unused_extras(&mut self) {
        let engine = &self.saved.engine;
        self.saved
            .extras
            .retain(|extra| engine.current.contains(&extra.id) || engine.history.iter().any(|frame| frame.contains(&extra.id)));
    }
}

fn write_atomically(file: &Path, contents: &str) -> std::io::Result<()> {
    if let Some(parent) = file.parent() {
        fs::create_dir_all(parent)?;
    }
    let temp = file.with_extension("json.part");
    fs::write(&temp, contents)?;
    fs::rename(&temp, file)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::model::{Schedule, WallpaperKind};

    fn ids(names: &[&str]) -> Vec<String> {
        names.iter().map(|name| name.to_string()).collect()
    }

    fn display(id: &str, x: f64, y: f64, width: f64, height: f64) -> DisplayInfo {
        DisplayInfo { id: id.into(), name: id.into(), x, y, width, height, scale: 2.0, primary: x == 0.0 && y == 0.0 }
    }

    fn two_displays() -> Vec<DisplayInfo> {
        // Listed right-first on purpose: reading order must not depend on list order.
        vec![display("right", 1512.0, 0.0, 2560.0, 1440.0), display("left", 0.0, 0.0, 1512.0, 982.0)]
    }

    fn image(id: &str) -> PlayItem {
        PlayItem { id: id.into(), kind: WallpaperKind::Image, path: format!("/lib/{id}.jpg"), poster_path: None }
    }

    fn saved(current: &[&str], layout: Layout) -> Saved {
        Saved { engine: EngineState { current: ids(current), ..Default::default() }, layout, ..Default::default() }
    }

    #[test]
    fn status_has_one_entry_without_a_display_for_same_and_span() {
        for layout in [Layout::Same, Layout::Span] {
            let status = status_of(&saved(&["a"], layout));
            assert_eq!(status.current, vec![CurrentEntry { display_id: None, item_id: "a".into() }]);
        }
        assert!(status_of(&Saved::default()).current.is_empty());
    }

    #[test]
    fn status_has_one_entry_per_display_for_each() {
        let status = status_of(&saved(&["a", "b"], Layout::PerDisplay(ids(&["left", "right"]))));
        assert_eq!(
            status.current,
            vec![
                CurrentEntry { display_id: Some("left".into()), item_id: "a".into() },
                CurrentEntry { display_id: Some("right".into()), item_id: "b".into() },
            ]
        );
    }

    #[test]
    fn status_reports_pause_schedule_and_history() {
        let mut state = saved(&["b"], Layout::Same);
        state.paused = true;
        state.next_change_at = None;
        state.engine.history = vec![ids(&["a"])];
        let status = status_of(&state);
        assert!(status.paused && status.can_go_previous);
        assert_eq!(status.next_change_at, None);
        state.engine.history.clear();
        state.next_change_at = Some(42);
        let status = status_of(&state);
        assert!(!status.can_go_previous);
        assert_eq!(status.next_change_at, Some(42));
    }

    #[test]
    fn frames_hold_one_wallpaper_or_one_per_display() {
        let displays = two_displays();
        assert_eq!(shape_for(DisplayMode::Same, &displays), (1, Layout::Same));
        assert_eq!(shape_for(DisplayMode::Span, &displays), (1, Layout::Span));
        assert_eq!(shape_for(DisplayMode::Each, &displays), (2, Layout::PerDisplay(ids(&["left", "right"]))));
    }

    #[test]
    fn same_puts_the_first_item_on_every_display() {
        let targets = plan_targets(&Layout::Same, &[image("a")], &two_displays());
        assert_eq!(targets.len(), 2);
        for target in &targets {
            assert_eq!(target.item.id, "a");
            assert!(!target.spanned);
            // Each window is its own canvas.
            assert_eq!((target.viewport.x, target.viewport.y), (0.0, 0.0));
            assert_eq!(target.viewport.canvas_width, target.display.width);
            assert_eq!(target.viewport.canvas_height, target.display.height);
        }
    }

    #[test]
    fn span_shares_one_canvas_and_gives_each_display_its_offset() {
        let targets = plan_targets(&Layout::Span, &[image("a")], &two_displays());
        let right = targets.iter().find(|target| target.display.id == "right").unwrap();
        let left = targets.iter().find(|target| target.display.id == "left").unwrap();
        for target in [left, right] {
            assert!(target.spanned);
            assert_eq!((target.viewport.canvas_width, target.viewport.canvas_height), (4072.0, 1440.0));
        }
        assert_eq!((left.viewport.x, left.viewport.y), (0.0, 0.0));
        assert_eq!((right.viewport.x, right.viewport.y), (1512.0, 0.0));
    }

    #[test]
    fn span_on_a_single_display_behaves_like_same() {
        let one = vec![display("only", 0.0, 0.0, 1512.0, 982.0)];
        let targets = plan_targets(&Layout::Span, &[image("a")], &one);
        assert_eq!(targets.len(), 1);
        assert!(!targets[0].spanned);
        assert_eq!(targets[0].viewport.canvas_width, 1512.0);
    }

    #[test]
    fn each_pairs_items_with_displays_by_id_and_skips_unplugged_ones() {
        let layout = Layout::PerDisplay(ids(&["left", "gone", "right"]));
        let targets = plan_targets(&layout, &[image("a"), image("b"), image("c")], &two_displays());
        let pairs: Vec<(&str, &str)> =
            targets.iter().map(|target| (target.display.id.as_str(), target.item.id.as_str())).collect();
        assert_eq!(pairs, [("left", "a"), ("right", "c")]);
    }

    #[test]
    fn history_frames_are_fitted_to_the_current_mode() {
        let displays = two_displays();
        assert_eq!(fit_frame(&ids(&["a", "b"]), DisplayMode::Same, &displays), (ids(&["a"]), Layout::Same));
        assert_eq!(fit_frame(&ids(&["a", "b"]), DisplayMode::Span, &displays), (ids(&["a"]), Layout::Span));
        let per_display = Layout::PerDisplay(ids(&["left", "right"]));
        assert_eq!(fit_frame(&ids(&["a", "b"]), DisplayMode::Each, &displays), (ids(&["a", "b"]), per_display.clone()));
        assert_eq!(fit_frame(&ids(&["a"]), DisplayMode::Each, &displays), (ids(&["a", "a"]), per_display));
    }

    #[test]
    fn the_per_display_view_follows_display_ids_not_positions() {
        let ordered = ids(&["left", "right"]);
        // Saved while the displays were the other way round.
        let swapped = Layout::PerDisplay(ids(&["right", "left"]));
        assert_eq!(per_display_view(&ids(&["a", "b"]), &swapped, &ordered), vec![Some("b".into()), Some("a".into())]);
        // A display the frame does not know yet.
        let partial = Layout::PerDisplay(ids(&["left"]));
        assert_eq!(per_display_view(&ids(&["a"]), &partial, &ordered), vec![Some("a".into()), None]);
        // "Same" means every display shows the one wallpaper.
        assert_eq!(per_display_view(&ids(&["a"]), &Layout::Same, &ordered), vec![Some("a".into()), Some("a".into())]);
        assert_eq!(per_display_view(&[], &Layout::Same, &ordered), vec![None, None]);
    }

    #[test]
    fn unknown_displays_are_filled_by_drawing_then_by_repeating() {
        let ordered = ids(&["left", "middle", "right"]);
        let view = vec![Some("a".to_string()), None, None];
        assert_eq!(
            fill_view(&ordered, view.clone(), &ids(&["x", "y"]), &ids(&["a"])),
            (ordered.clone(), ids(&["a", "x", "y"]))
        );
        assert_eq!(fill_view(&ordered, view.clone(), &ids(&["x"]), &ids(&["a", "b"])), (ordered.clone(), ids(&["a", "x", "a"])));
        // With nothing at all to put there, the display is left out and the two lists still line up.
        assert_eq!(fill_view(&ordered, view, &[], &[]), (ids(&["left"]), ids(&["a"])));
    }

    /// The whole "only on this display" rule on the pure state: two displays
    /// showing [a, b], c applied to the second one.
    #[test]
    fn applying_to_one_display_replaces_only_its_entry() {
        let ordered = ids(&["left", "right"]);
        let before = Saved {
            engine: EngineState { bag: ids(&["c", "d"]), current: ids(&["a", "b"]), history: vec![] },
            layout: Layout::PerDisplay(ordered.clone()),
            ..Default::default()
        };
        let view = per_display_view(&before.engine.current, &before.layout, &ordered);
        let (display_ids, frame) = fill_view(&ordered, view, &[], &ids(&["c"]));
        assert_eq!(frame, ids(&["a", "b"]), "a complete frame needs no topping up");
        let staged = EngineState { current: frame, ..before.engine.clone() };
        let slot = ordered.iter().position(|id| id == "right").unwrap();
        let after = Saved {
            engine: engine::show_in_slot(&staged, slot, "c"),
            layout: Layout::PerDisplay(display_ids),
            ..Default::default()
        };

        assert_eq!(after.engine.current, ids(&["a", "c"]));
        assert_eq!(after.engine.history, vec![ids(&["a", "b"])]);
        let status = status_of(&after);
        assert_eq!(
            status.current,
            vec![
                CurrentEntry { display_id: Some("left".into()), item_id: "a".into() },
                CurrentEntry { display_id: Some("right".into()), item_id: "c".into() },
            ]
        );
        assert!(status.can_go_previous);

        // And only that display is asked to change.
        let only = Layout::PerDisplay(vec![ordered[slot].clone()]);
        let targets = plan_targets(&only, &[image("c")], &two_displays());
        assert_eq!(targets.len(), 1);
        assert_eq!((targets[0].display.id.as_str(), targets[0].item.id.as_str()), ("right", "c"));
    }

    #[test]
    fn reconcile_keeps_a_per_display_layout_lined_up() {
        let mut state = saved(&["a", "gone", "c"], Layout::PerDisplay(ids(&["d1", "d2", "d3"])));
        state.engine.history = vec![ids(&["gone"]), ids(&["a", "gone"])];
        reconcile_saved(&mut state, &ids(&["a", "c"]));
        assert_eq!(state.engine.current, ids(&["a", "c"]));
        assert_eq!(state.layout, Layout::PerDisplay(ids(&["d1", "d3"])));
        assert_eq!(state.engine.history, vec![ids(&["a"])]);
        let status = status_of(&state);
        assert_eq!(status.current[1], CurrentEntry { display_id: Some("d3".into()), item_id: "c".into() });
    }

    #[test]
    fn saved_state_round_trips_and_tolerates_missing_fields() {
        let state = Saved {
            plan: Some(RotationPlan {
                items: vec![image("a")],
                shuffle: true,
                schedule: Schedule::Daily { time: "09:00".into() },
                paused: false,
                display_mode: DisplayMode::Each,
                pause_live_on_battery: true,
            }),
            paused: false,
            engine: EngineState { bag: ids(&["a"]), current: ids(&["a", "b"]), history: vec![ids(&["c"])] },
            layout: Layout::PerDisplay(ids(&["1", "2"])),
            next_change_at: Some(1_800_000_000_000),
            extras: vec![image("b")],
        };
        let json = serde_json::to_string(&state).unwrap();
        assert_eq!(serde_json::from_str::<Saved>(&json).unwrap(), state);
        // An empty or older file still loads.
        assert_eq!(serde_json::from_str::<Saved>("{}").unwrap(), Saved::default());
        let partial: Saved = serde_json::from_str(r#"{"paused":true,"layout":{"kind":"span"}}"#).unwrap();
        assert!(partial.paused);
        assert_eq!(partial.layout, Layout::Span);
    }

    #[test]
    fn state_files_are_replaced_whole() {
        let dir = std::env::temp_dir().join(format!("still-rotation-test-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        let file = dir.join("nested").join("rotation.json");
        write_atomically(&file, "{\"paused\":true}").unwrap();
        write_atomically(&file, "{\"paused\":false}").unwrap();
        assert!(!load(&file).paused);
        assert_eq!(fs::read_dir(file.parent().unwrap()).unwrap().count(), 1, "no temporary file left behind");
        fs::write(&file, "not json").unwrap();
        assert_eq!(load(&file), Saved::default());
        assert_eq!(load(&dir.join("absent.json")), Saved::default());
        let _ = fs::remove_dir_all(&dir);
    }
}
