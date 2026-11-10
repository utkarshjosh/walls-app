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
//!
//! Automatic changes only use wallpapers large enough for the displays they
//! would go on, and a wallpaper is stretched across displays only when it has
//! the pixels for it (the rules are in `fit.rs`). A wallpaper chosen by hand
//! is shown regardless.

use std::collections::{HashMap, HashSet};
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::mpsc::{self, Receiver, RecvTimeoutError, Sender, SyncSender};
use std::sync::{Arc, Mutex, PoisonError};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter};

use crate::displays;
use crate::engine::{self, EngineState, Rng};
use crate::fit::{self, FitItem, Placement};
use crate::library::Library;
use crate::live::{LiveTarget, LiveWindows, Viewport};
use crate::model::{
    ApplyRequest, CurrentEntry, DisplayInfo, DisplayMode, PlayItem, RotationCommand, RotationPlan, RotationStatus,
    WallpaperKind,
};
use crate::tray::Tray;
use crate::{power, scene, schedule, wallpaper, MAIN_WINDOW};

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
    /// An entry of "" means Still has put nothing on that display.
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
        // A "" entry is a display Still has put nothing on: it is left out.
        Layout::PerDisplay(display_ids) => display_ids
            .iter()
            .zip(&saved.engine.current)
            .filter(|(_, item_id)| !item_id.is_empty())
            .map(|(display_id, item_id)| CurrentEntry { display_id: Some(display_id.clone()), item_id: item_id.clone() })
            .collect(),
        Layout::Same | Layout::Span => saved
            .engine
            .current
            .iter()
            .find(|item_id| !item_id.is_empty())
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

/// How many wallpapers a frame holds, and how it is laid out, for a display
/// mode. For "span" this is the layout asked for; `single_layout` decides
/// whether a particular wallpaper can really be spanned.
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

/// What the size rules judge a wallpaper by: the size the UI sent, or failing
/// that the size read from the file (`measured`), and whether it is a scene
/// that draws itself whole on each display. Without either size it is
/// "unknown", which passes.
pub fn fit_item(item: &PlayItem, measured: Option<(u32, u32)>) -> FitItem {
    let (width, height) = if item.width > 0.0 && item.height > 0.0 {
        (item.width, item.height)
    } else {
        measured.map_or((0.0, 0.0), |(width, height)| (f64::from(width), f64::from(height)))
    };
    FitItem { kind: item.kind, width, height, per_display: item.per_display }
}

/// The layout for a one-wallpaper frame: stretched across the displays only
/// when that is wanted and this wallpaper has the pixels for it; otherwise it
/// is shown on each display. An unknown wallpaper is never stretched.
pub fn single_layout(want_span: bool, item: Option<&FitItem>, displays: &[DisplayInfo]) -> Layout {
    if want_span && item.is_some_and(|item| fit::can_span(item, displays)) {
        Layout::Span
    } else {
        Layout::Same
    }
}

/// The wallpapers automatic rotation may use when every display shows the same
/// thing ("same") or one picture is stretched across all ("span"): everything
/// except what is too small for at least one display. Keeps the given order.
pub fn eligible_ids(candidates: &[(String, FitItem)], displays: &[DisplayInfo], mode: DisplayMode) -> Vec<String> {
    candidates
        .iter()
        .filter(|(_, item)| fit::placement_for(item, displays, mode) != Placement::Skip)
        .map(|(id, _)| id.clone())
        .collect()
}

/// For "different on each display": the wallpapers large enough for each
/// display, in the order of `ordered`.
pub fn allowed_per_display(candidates: &[(String, FitItem)], ordered: &[DisplayInfo]) -> Vec<HashSet<String>> {
    ordered
        .iter()
        .map(|display| {
            candidates.iter().filter(|(_, item)| fit::fits_display(item, display)).map(|(id, _)| id.clone()).collect()
        })
        .collect()
}

/// The next automatic frame and how to lay it out, or `None` when nothing is
/// large enough for these displays (then nothing changes).
///
/// `candidates` are the plan's usable wallpapers with their sizes, in plan
/// order; `ordered` are the connected displays in reading order.
///  - same: only wallpapers that fit every display.
///  - span: also only those, but one that can be stretched is stretched and
///    one that cannot is shown on each display for its turn.
///  - each: every display draws from the wallpapers large enough for it; a
///    display with none gets "" and is left alone.
pub fn next_frame(
    state: &EngineState,
    mode: DisplayMode,
    candidates: &[(String, FitItem)],
    ordered: &[DisplayInfo],
    shuffle: bool,
    random: &mut dyn FnMut() -> f64,
) -> Option<(EngineState, Layout)> {
    if mode == DisplayMode::Each {
        let allowed = allowed_per_display(candidates, ordered);
        if allowed.iter().all(HashSet::is_empty) {
            return None;
        }
        let ids: Vec<String> = candidates.iter().map(|(id, _)| id.clone()).collect();
        let next = engine::advance_slots(state, &ids, &allowed, shuffle, random);
        let display_ids = ordered.iter().map(|display| display.id.clone()).collect();
        return Some((next, Layout::PerDisplay(display_ids)));
    }
    let eligible = eligible_ids(candidates, ordered, mode);
    if eligible.is_empty() {
        return None;
    }
    let next = engine::advance(state, &eligible, shuffle, 1, random);
    let size = next.current.first().and_then(|id| candidates.iter().find(|(known, _)| known == id)).map(|(_, size)| size);
    let layout = single_layout(mode == DisplayMode::Span, size, ordered);
    Some((next, layout))
}

/// Which item goes on which display. `items` lines up with the frame; `None`
/// is a "" entry, a display that is left exactly as it is.
pub fn plan_targets(layout: &Layout, items: &[Option<PlayItem>], displays: &[DisplayInfo]) -> Vec<Target> {
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
                let item = item.as_ref()?;
                let display = displays.iter().find(|display| display.id == *display_id)?;
                Some(Target { display: display.clone(), item: item.clone(), viewport: own_screen(display), spanned: false })
            })
            .collect(),
        Layout::Same | Layout::Span => {
            let Some(item) = items.iter().flatten().next() else {
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

/// The frame as one entry per display in `ordered`, "" where it has nothing on
/// that display. A per-display frame is read by display id. A one-wallpaper
/// frame (same or span) is on the displays listed in `on`: normally the ones
/// it was last laid out for, or just the first display when the others are
/// about to be given wallpapers of their own.
pub fn per_display_view(current: &[String], layout: &Layout, ordered: &[String], on: &[String]) -> Vec<String> {
    let single = current.iter().find(|id| !id.is_empty()).cloned().unwrap_or_default();
    ordered
        .iter()
        .map(|display_id| match layout {
            Layout::PerDisplay(display_ids) => display_ids
                .iter()
                .position(|known| known == display_id)
                .and_then(|slot| current.get(slot).cloned())
                .unwrap_or_default(),
            Layout::Same | Layout::Span if on.contains(display_id) => single.clone(),
            Layout::Same | Layout::Span => String::new(),
        })
        .collect()
}

/// A frame from history laid over `count` displays, position by position: a
/// one-wallpaper frame was on every display; a per-display frame keeps its
/// positions, and a display it has no entry for is a gap.
pub fn history_view(frame: &[String], count: usize) -> Vec<String> {
    match frame {
        [single] => vec![single.clone(); count],
        _ => (0..count).map(|slot| frame.get(slot).cloned().unwrap_or_default()).collect(),
    }
}

/// From what each display shows (`view`), the one wallpaper all of them could
/// show without it newly landing on a display it is too small for: the first
/// that every display either already shows or is large enough for
/// (`fits(id, slot)`), or that can be stretched across them all (`spans`).
/// `None` when there is no such wallpaper; the caller then moves on to a
/// wallpaper that does fit, or leaves the displays as they are. A re-layout
/// must never put a too-small picture on a display by itself.
pub fn keep_for_all(view: &[String], fits: impl Fn(&str, usize) -> bool, spans: impl Fn(&str) -> bool) -> Option<String> {
    view.iter()
        .filter(|id| !id.is_empty())
        .find(|candidate| {
            spans(candidate)
                || view.iter().enumerate().all(|(slot, showing)| showing == *candidate || fits(candidate, slot))
        })
        .cloned()
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
        measured: HashMap::new(),
        laid_out: Vec::new(),
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
    /// Picture sizes read from files for wallpapers that arrived without one, by path.
    measured: HashMap<String, Measured>,
    /// The displays the current frame was last put on. A display connected
    /// since is not in it, which is how "would newly appear there" is known
    /// for a one-wallpaper frame.
    laid_out: Vec<String>,
    /// The display list differs from the one the current frame was last laid
    /// out for; `check_displays` re-applies and clears it.
    arrangement_changed: bool,
    displays_dirty_since: Option<Instant>,
    last_display_check: Instant,
    last_power_check: Instant,
}

/// A size read from a file, with the file's length and modification time so a
/// file replaced under the same name is measured again.
struct Measured {
    stamp: (u64, u128),
    size: Option<(u32, u32)>,
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

/// What to do with displays a per-display frame says nothing about.
#[derive(Clone, Copy, PartialEq)]
enum Gaps {
    /// Leave them out: Still has put nothing on them.
    Leave,
    /// Draw a wallpaper large enough for each from the plan; a display with
    /// nothing large enough stays out.
    Draw,
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
        self.laid_out = self.displays.iter().map(|display| display.id.clone()).collect();

        // A live wallpaper does not survive a restart by itself: the desktop
        // keeps only its poster. Bring the windows back; no picture is changed.
        let frame = self.saved.engine.current.clone();
        let layout = self.saved.layout.clone();
        let has_live = frame.iter().any(|id| self.item(id).is_some_and(|item| item.kind.is_live()));
        if has_live {
            if let Err(error) = self.show_frame(&frame, &layout, Part::LiveOnly) {
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
                let never_shown = !engine::has_any(&self.saved.engine.current);
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
        // Frames keep their positions, so a per-display layout stays lined up.
        self.saved.engine = engine::reconcile(&self.saved.engine, &alive);
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
        if mode_changed && engine::has_any(&self.saved.engine.current) {
            if let Err(error) = self.reshape(plan.display_mode) {
                eprintln!("still: could not re-apply the wallpaper in the new display mode: {error}");
            }
        } else if self.saved.layout == Layout::Span {
            // A scene that draws itself whole on each display is never one
            // picture across displays. If it is recorded as one (saved before
            // the plan said which scenes are like that), lay it out, and its
            // poster underneath, per display, so the record matches the screen.
            let frame = self.saved.engine.current.clone();
            let whole_on_each = frame.iter().find(|id| !id.is_empty()).and_then(|id| self.item(id));
            if whole_on_each.is_some_and(|item| item.per_display) {
                match self.show_frame(&frame, &Layout::Same, Part::Everything) {
                    Ok(()) => self.saved.layout = Layout::Same,
                    Err(error) => eprintln!("still: could not lay the scene out per display: {}", error.message),
                }
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
                if has_items && !engine::has_any(&self.saved.engine.current) {
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

    /// A wallpaper chosen by hand. Size is not checked: the person asked for
    /// it. The one exception is "span", where a wallpaper too small to stretch
    /// is shown on each display instead.
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
            // The other displays keep what they show; one Still has put
            // nothing on stays a gap rather than being claimed.
            let on = self.laid_out.clone();
            let (staged, layout) = self.per_display_state(&on, Gaps::Leave);
            let next = engine::show_in_slot(&staged, slot, &first.id);
            let only = Layout::PerDisplay(vec![ordered[slot].id.clone()]);
            let targets = plan_targets(&only, &[Some(first.clone())], &self.displays);
            (next, layout, targets)
        } else {
            let (count, wanted) = shape_for(request.mode, &self.displays);
            let layout = match wanted {
                Layout::Span => {
                    let size = self.size_of(&first);
                    single_layout(true, Some(&size), &self.displays)
                }
                other => other,
            };
            let items: Vec<PlayItem> =
                (0..count).map(|slot| request.items[slot % request.items.len()].clone()).collect();
            let frame: Vec<String> = items.iter().map(|item| item.id.clone()).collect();
            let items: Vec<Option<PlayItem>> = items.into_iter().map(Some).collect();
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

    /// Automatic or "next": draw the next frame and show it.
    ///
    /// Only wallpapers large enough for where they would go are drawn; see
    /// `next_frame`. A wallpaper whose file is missing or unusable is skipped
    /// in favour of another, each at most once, so one bad file never stops
    /// rotation or blanks the desktop.
    ///
    /// `Ok(false)` means nothing changed because there was nothing to rotate,
    /// or nothing large enough; that is not an error and the schedule carries on.
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
        let ordered = displays::reading_order(&self.displays);
        let mut usable = self.usable_ids(&plan);
        if usable.is_empty() {
            return Err(NOTHING_USABLE.to_string());
        }
        let sizes: HashMap<String, FitItem> =
            plan.items.iter().map(|item| (item.id.clone(), self.size_of(item))).collect();
        let mut last_error: Option<String> = None;

        loop {
            let candidates: Vec<(String, FitItem)> =
                usable.iter().filter_map(|id| sizes.get(id).map(|size| (id.clone(), *size))).collect();
            let drawn = {
                let rng = &mut self.rng;
                next_frame(&self.saved.engine, plan.display_mode, &candidates, &ordered, plan.shuffle, &mut || {
                    rng.next_f64()
                })
            };
            let Some((next, layout)) = drawn else {
                // Nothing is large enough. If that is only because a file just
                // failed, say so; otherwise leave everything as it is.
                return last_error.map_or(Ok(false), Err);
            };
            match self.show_frame(&next.current, &layout, Part::Everything) {
                Ok(()) => {
                    self.saved.engine = next;
                    self.saved.layout = layout;
                    return Ok(true);
                }
                Err(ShowError { message, item_id: Some(bad) }) => {
                    usable.retain(|id| *id != bad);
                    last_error = Some(message);
                }
                Err(error) => return Err(error.message),
            }
        }
    }

    /// Step back through what was really shown. History entries that can no
    /// longer be shown (file deleted since) are dropped and the one before is
    /// tried. Size is not checked for a wallpaper going back where it was.
    fn previous(&mut self) -> Result<bool, String> {
        self.refresh_displays();
        let mode = self.saved.plan.as_ref().map_or(DisplayMode::Same, |plan| plan.display_mode);
        let ordered = displays::reading_order(&self.displays);
        let display_ids: Vec<String> = ordered.iter().map(|display| display.id.clone()).collect();
        let mut last_error = None;
        while let Some(stepped) = engine::back(&self.saved.engine) {
            let view = history_view(&stepped.current, ordered.len());
            // With one wallpaper for every display, an older per-display frame
            // is reduced to one that can go everywhere. If none can, each
            // display simply gets back what it showed.
            let one = if mode == DisplayMode::Each { None } else { self.keeper(&view, &ordered, mode) };
            let (frame, layout) = match one {
                Some(id) => {
                    let frame = vec![id];
                    let layout = self.settle(&frame, shape_for(mode, &self.displays).1);
                    (frame, layout)
                }
                None => (view, Layout::PerDisplay(display_ids.clone())),
            };
            match self.show_frame(&frame, &layout, Part::Everything) {
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
        if mode != DisplayMode::Each {
            return self.relayout_as_one(mode);
        }
        // The first display keeps the wallpaper; the others each get one large
        // enough for them, or are left as they are.
        let first: Vec<String> =
            displays::reading_order(&self.displays).into_iter().take(1).map(|display| display.id).collect();
        let (next, layout) = self.per_display_state(&first, Gaps::Draw);
        self.show_frame(&next.current, &layout, Part::Everything).map_err(|error| error.message)?;
        self.saved.engine = next;
        self.saved.layout = layout;
        Ok(())
    }

    /// Bring the desktop to one wallpaper on every display (mode same or
    /// span), after a mode change or a change of displays, without ever
    /// putting a wallpaper on a display it is too small for:
    ///  1. keep a wallpaper that is showing, if it can go on every display
    ///     (see `keep_for_all`); this is not a change, so no history entry;
    ///  2. otherwise make one automatic change, exactly as "Next" would;
    ///  3. otherwise leave every display as it is. The frame then records what
    ///     is on each display, so the status may list displays one by one
    ///     although the plan says same or span.
    fn relayout_as_one(&mut self, mode: DisplayMode) -> Result<(), String> {
        let ordered = displays::reading_order(&self.displays);
        let display_ids: Vec<String> = ordered.iter().map(|display| display.id.clone()).collect();
        let view = per_display_view(&self.saved.engine.current, &self.saved.layout, &display_ids, &self.laid_out);

        if let Some(keep) = self.keeper(&view, &ordered, mode) {
            let frame = vec![keep];
            let layout = self.settle(&frame, shape_for(mode, &self.displays).1);
            self.show_frame(&frame, &layout, Part::Everything).map_err(|error| error.message)?;
            self.saved.engine.current = frame;
            self.saved.layout = layout;
            return Ok(());
        }
        match self.advance() {
            Ok(true) => return Ok(()),
            Ok(false) => {}
            Err(error) => eprintln!("still: no automatic change was possible: {error}"),
        }
        if engine::has_any(&view) {
            self.saved.engine.current = view;
            self.saved.layout = Layout::PerDisplay(display_ids.clone());
        }
        // Nothing is shown anew, but windows of displays that went away must close.
        let _ = self.live.sync(&self.app, &[], &[], &display_ids, false);
        Ok(())
    }

    /// `keep_for_all` with the real sizes of the wallpapers in `view`.
    fn keeper(&mut self, view: &[String], ordered: &[DisplayInfo], mode: DisplayMode) -> Option<String> {
        let mut sizes: HashMap<String, FitItem> = HashMap::new();
        for id in view.iter().filter(|id| !id.is_empty()) {
            if let Some(item) = self.item(id) {
                sizes.insert(id.clone(), self.size_of(&item));
            }
        }
        keep_for_all(
            view,
            |id, slot| sizes.get(id).is_some_and(|size| fit::fits_display(size, &ordered[slot])),
            |id| mode == DisplayMode::Span && sizes.get(id).is_some_and(|size| fit::can_span(size, ordered)),
        )
    }

    /// Turn a layout that asks for spanning into what this frame's wallpaper
    /// can actually do on the displays connected now.
    fn settle(&mut self, frame: &[String], wanted: Layout) -> Layout {
        if wanted != Layout::Span {
            return wanted;
        }
        let item = frame.iter().find(|id| !id.is_empty()).and_then(|id| self.item(id));
        let size = item.map(|item| self.size_of(&item));
        single_layout(true, size.as_ref(), &self.displays)
    }

    /// The current frame as one entry per connected display (reading order),
    /// with `gaps` deciding what happens to displays it has nothing on. `on`
    /// is explained at `per_display_view`. Not a change in itself: no history
    /// entry.
    fn per_display_state(&mut self, on: &[String], gaps: Gaps) -> (EngineState, Layout) {
        let ordered = displays::reading_order(&self.displays);
        let display_ids: Vec<String> = ordered.iter().map(|display| display.id.clone()).collect();
        let kept = per_display_view(&self.saved.engine.current, &self.saved.layout, &display_ids, on);

        let mut staged = self.saved.engine.clone();
        let plan = self.saved.plan.clone().filter(|_| gaps == Gaps::Draw && kept.iter().any(String::is_empty));
        if let Some(plan) = plan {
            let usable = self.usable_ids(&plan);
            let candidates: Vec<(String, FitItem)> = plan
                .items
                .iter()
                .filter(|item| usable.contains(&item.id))
                .map(|item| (item.id.clone(), self.size_of(item)))
                .collect();
            let allowed = allowed_per_display(&candidates, &ordered);
            let rng = &mut self.rng;
            staged = engine::fill_slots(&staged, &kept, &usable, &allowed, plan.shuffle, &mut || rng.next_f64());
        } else {
            staged.current = kept;
        }
        (staged, Layout::PerDisplay(display_ids))
    }

    fn show_frame(&mut self, frame: &[String], layout: &Layout, part: Part) -> Result<(), ShowError> {
        let mut items = Vec::with_capacity(frame.len());
        for id in frame {
            if id.is_empty() {
                // A display that is left exactly as it is.
                items.push(None);
                continue;
            }
            match self.item(id) {
                Some(item) => items.push(Some(item)),
                None => return Err(ShowError::about(id, "That wallpaper is no longer available.")),
            }
        }
        let targets = plan_targets(layout, &items, &self.displays);
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
            // A scene is loaded as a web page. Only files stored under a scene
            // name are checked for the content policy when they arrive, so
            // only those may be played as one; a picture or video file handed
            // over as a "scene" is not. (The name is all that is looked at
            // here: scenes saved before the check existed keep playing.)
            if target.item.kind == WallpaperKind::Scene && !scene::is_scene_name(&target.item.path) {
                return Err(ShowError::about(&target.item.id, scene::REFUSAL));
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
        // Displays that are not targets (a "" entry, or the others when one
        // display is changed by hand) are outside the scope and keep whatever
        // they show, live window included.
        let scope: Vec<String> = targets.iter().map(|target| target.display.id.clone()).collect();
        let connected: Vec<String> = self.displays.iter().map(|display| display.id.clone()).collect();
        let paused = self.live_should_pause();
        if let Err(error) = self.live.sync(&self.app, &wanted, &scope, &connected, paused) {
            // The poster is already on the desktop, so the change itself stands.
            eprintln!("still: live wallpaper problem: {error}");
        }
        self.last_power_check = Instant::now();
        if part == Part::Everything {
            let shown: Vec<String> = targets.iter().map(|target| target.display.id.clone()).collect();
            if connected.iter().all(|id| shown.contains(id)) {
                self.laid_out = shown;
            } else {
                // One display changed by hand, or a frame with gaps.
                for id in shown {
                    if !self.laid_out.contains(&id) {
                        self.laid_out.push(id);
                    }
                }
            }
        }
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
                let displays = &self.displays;
                self.laid_out.retain(|id| displays.iter().any(|display| display.id == *id));
                let _ = self.app.emit_to(MAIN_WINDOW, "displays-changed", &self.displays);
            }
            Ok(_) => {}
            Err(error) => eprintln!("still: could not read the displays: {error}"),
        }
    }

    /// Displays may have been added, removed or rearranged: tell the UI and
    /// put the current frame onto the new arrangement. A wallpaper stays on
    /// the displays it is on even if it is now too small for them, but it is
    /// not put on a newly connected display it is too small for.
    fn check_displays(&mut self) {
        self.refresh_displays();
        // With every display gone (lid closed, nothing attached) there is
        // nothing to lay out; their return counts as a new change.
        if !std::mem::take(&mut self.arrangement_changed) || self.displays.is_empty() {
            return;
        }
        if !engine::has_any(&self.saved.engine.current) {
            // Nothing of ours to re-apply; just drop windows for vanished displays.
            let connected: Vec<String> = self.displays.iter().map(|display| display.id.clone()).collect();
            let _ = self.live.sync(&self.app, &[], &[], &connected, false);
            return;
        }
        let plan_mode = self.saved.plan.as_ref().map(|plan| plan.display_mode);
        let per_display = matches!(self.saved.layout, Layout::PerDisplay(_));
        let outcome = if plan_mode == Some(DisplayMode::Each) || (plan_mode.is_none() && per_display) {
            // Each display keeps its wallpaper. When the plan gives every
            // display its own, a newly connected one gets a pick large enough
            // for it; without such a plan it is left alone.
            let gaps = if plan_mode == Some(DisplayMode::Each) { Gaps::Draw } else { Gaps::Leave };
            let on = self.laid_out.clone();
            let (next, layout) = self.per_display_state(&on, gaps);
            match self.show_frame(&next.current, &layout, Part::Everything) {
                Ok(()) => {
                    self.saved.engine = next;
                    self.saved.layout = layout;
                    Ok(())
                }
                Err(error) => Err(error.message),
            }
        } else {
            // One wallpaper everywhere. Stretched only if that is what is
            // wanted and it can be stretched over the displays there are now.
            let wants_span = self.saved.layout == Layout::Span || plan_mode == Some(DisplayMode::Span);
            self.relayout_as_one(if wants_span { DisplayMode::Span } else { DisplayMode::Same })
        };
        if let Err(error) = outcome {
            eprintln!("still: could not re-apply after a display change: {error}");
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

    /// What the size rules should judge this wallpaper by: the size the UI
    /// sent, or for a picture that arrived without one (state saved by an
    /// older build) the size read from the file. Still unknown means it passes.
    fn size_of(&mut self, item: &PlayItem) -> FitItem {
        let known = item.width > 0.0 && item.height > 0.0;
        let measured = if !known && item.kind == WallpaperKind::Image { self.measure(&item.path) } else { None };
        fit_item(item, measured)
    }

    /// The picture's size from its file, remembered until the file changes.
    fn measure(&mut self, path: &str) -> Option<(u32, u32)> {
        let metadata = fs::metadata(path).ok()?;
        let modified = metadata
            .modified()
            .ok()
            .and_then(|time| time.duration_since(UNIX_EPOCH).ok())
            .map_or(0, |since| since.as_nanos());
        let stamp = (metadata.len(), modified);
        if let Some(known) = self.measured.get(path).filter(|known| known.stamp == stamp) {
            return known.size;
        }
        let size = wallpaper::picture_size(Path::new(path));
        self.measured.insert(path.to_string(), Measured { stamp, size });
        size
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
    use crate::model::Schedule;

    fn ids(names: &[&str]) -> Vec<String> {
        names.iter().map(|name| name.to_string()).collect()
    }

    fn screen(id: &str, x: f64, y: f64, width: f64, height: f64, scale: f64) -> DisplayInfo {
        DisplayInfo { id: id.into(), name: id.into(), x, y, width, height, scale, primary: x == 0.0 && y == 0.0 }
    }

    fn display(id: &str, x: f64, y: f64, width: f64, height: f64) -> DisplayInfo {
        screen(id, x, y, width, height, 2.0)
    }

    fn two_displays() -> Vec<DisplayInfo> {
        // Listed right-first on purpose: reading order must not depend on list order.
        vec![display("right", 1512.0, 0.0, 2560.0, 1440.0), display("left", 0.0, 0.0, 1512.0, 982.0)]
    }

    /// The desk from the size rules' tests: a 16-inch MacBook Pro (3456 x 2234
    /// pixels) to the left of a 1080p monitor, in reading order.
    fn desk() -> Vec<DisplayInfo> {
        vec![screen("mac", -1728.0, 0.0, 1728.0, 1117.0, 2.0), screen("monitor", 0.0, 0.0, 1920.0, 1080.0, 1.0)]
    }

    fn image(id: &str) -> PlayItem {
        PlayItem {
            id: id.into(),
            kind: WallpaperKind::Image,
            path: format!("/lib/{id}.jpg"),
            width: 0.0,
            height: 0.0,
            per_display: false,
            poster_path: None,
        }
    }

    fn sized(width: f64, height: f64) -> FitItem {
        FitItem { kind: WallpaperKind::Image, width, height, per_display: false }
    }

    /// A scene with a poster, as the UI sends it; `per_display` is the clock kind.
    fn scene_item(id: &str, per_display: bool) -> PlayItem {
        PlayItem {
            id: id.into(),
            kind: WallpaperKind::Scene,
            path: format!("/lib/{id}.html"),
            width: 1920.0,
            height: 1200.0,
            per_display,
            poster_path: Some(format!("/lib/{id}.jpg")),
        }
    }

    /// A small library in plan order: a panorama, a 4K picture, the 1672 x 941
    /// sample that started all this, and a 720p picture.
    fn library() -> Vec<(String, FitItem)> {
        vec![
            ("panorama".to_string(), sized(7680.0, 2400.0)),
            ("fourk".to_string(), sized(3840.0, 2400.0)),
            ("sample".to_string(), sized(1672.0, 941.0)),
            ("hd720".to_string(), sized(1280.0, 720.0)),
        ]
    }

    fn only(names: &[&str]) -> Vec<(String, FitItem)> {
        library().into_iter().filter(|(id, _)| names.contains(&id.as_str())).collect()
    }

    fn rng(seed: u64) -> impl FnMut() -> f64 {
        let mut source = Rng::from_seed(seed);
        move || source.next_f64()
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
    fn status_leaves_out_displays_still_has_put_nothing_on() {
        let status = status_of(&saved(&["", "b"], Layout::PerDisplay(ids(&["left", "right"]))));
        assert_eq!(status.current, vec![CurrentEntry { display_id: Some("right".into()), item_id: "b".into() }]);
        assert!(status_of(&saved(&["", ""], Layout::PerDisplay(ids(&["left", "right"])))).current.is_empty());
        assert!(status_of(&saved(&[""], Layout::Same)).current.is_empty());
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
        let targets = plan_targets(&Layout::Same, &[Some(image("a"))], &two_displays());
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
        let targets = plan_targets(&Layout::Span, &[Some(image("a"))], &two_displays());
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
        let targets = plan_targets(&Layout::Span, &[Some(image("a"))], &one);
        assert_eq!(targets.len(), 1);
        assert!(!targets[0].spanned);
        assert_eq!(targets[0].viewport.canvas_width, 1512.0);
    }

    #[test]
    fn each_pairs_items_with_displays_by_id_and_skips_unplugged_ones() {
        let layout = Layout::PerDisplay(ids(&["left", "gone", "right"]));
        let items = [Some(image("a")), Some(image("b")), Some(image("c"))];
        let targets = plan_targets(&layout, &items, &two_displays());
        let pairs: Vec<(&str, &str)> =
            targets.iter().map(|target| (target.display.id.as_str(), target.item.id.as_str())).collect();
        assert_eq!(pairs, [("left", "a"), ("right", "c")]);
    }

    #[test]
    fn a_display_with_no_entry_is_not_a_target() {
        let layout = Layout::PerDisplay(ids(&["left", "right"]));
        let targets = plan_targets(&layout, &[None, Some(image("c"))], &two_displays());
        assert_eq!(targets.len(), 1);
        assert_eq!((targets[0].display.id.as_str(), targets[0].item.id.as_str()), ("right", "c"));
        assert!(plan_targets(&layout, &[None, None], &two_displays()).is_empty());
        assert!(plan_targets(&Layout::Same, &[None], &two_displays()).is_empty());
    }

    #[test]
    fn a_frame_from_history_is_laid_over_todays_displays_by_position() {
        // One wallpaper was on every display.
        assert_eq!(history_view(&ids(&["a"]), 2), ids(&["a", "a"]));
        // A per-display frame keeps its positions; gaps stay gaps.
        assert_eq!(history_view(&ids(&["a", "b"]), 2), ids(&["a", "b"]));
        assert_eq!(history_view(&ids(&["", "b"]), 2), ids(&["", "b"]));
        // A display connected since has no entry; one that is gone drops off.
        assert_eq!(history_view(&ids(&["a", "b"]), 3), ids(&["a", "b", ""]));
        assert_eq!(history_view(&ids(&["a", "b", "c"]), 2), ids(&["a", "b"]));
        assert_eq!(history_view(&ids(&["a"]), 0), Vec::<String>::new());
    }

    #[test]
    fn the_per_display_view_follows_display_ids_not_positions() {
        let ordered = ids(&["left", "right"]);
        // Saved while the displays were the other way round.
        let swapped = Layout::PerDisplay(ids(&["right", "left"]));
        assert_eq!(per_display_view(&ids(&["a", "b"]), &swapped, &ordered, &[]), ids(&["b", "a"]));
        // A display the frame does not know yet, and one it has nothing on.
        let partial = Layout::PerDisplay(ids(&["left"]));
        assert_eq!(per_display_view(&ids(&["a"]), &partial, &ordered, &ordered), ids(&["a", ""]));
        let gap = Layout::PerDisplay(ids(&["left", "right"]));
        assert_eq!(per_display_view(&ids(&["", "b"]), &gap, &ordered, &ordered), ids(&["", "b"]));
    }

    #[test]
    fn one_wallpaper_is_on_the_displays_it_was_put_on() {
        let ordered = ids(&["left", "right"]);
        for layout in [Layout::Same, Layout::Span] {
            // Laid out for both displays: both show it.
            assert_eq!(per_display_view(&ids(&["a"]), &layout, &ordered, &ordered), ids(&["a", "a"]));
            // "right" was connected afterwards: it shows nothing of ours yet.
            assert_eq!(per_display_view(&ids(&["a"]), &layout, &ordered, &ids(&["left"])), ids(&["a", ""]));
            assert_eq!(per_display_view(&ids(&["a"]), &layout, &ordered, &ids(&["right"])), ids(&["", "a"]));
            assert_eq!(per_display_view(&[], &layout, &ordered, &ordered), ids(&["", ""]));
        }
    }

    // --- re-layouts never put a too-small wallpaper on a display by themselves ---

    /// `keep_for_all` on the desk, with the library's real sizes.
    fn keep_on_desk(view: &[&str], span: bool) -> Option<String> {
        let sizes = library();
        let size = |id: &str| sizes.iter().find(|(known, _)| known == id).map(|(_, size)| *size);
        keep_for_all(
            &ids(view),
            |id, slot| size(id).is_some_and(|size| fit::fits_display(&size, &desk()[slot])),
            |id| span && size(id).is_some_and(|size| fit::can_span(&size, &desk())),
        )
    }

    #[test]
    fn a_wallpaper_already_everywhere_is_kept_whatever_its_size() {
        // Chosen by hand earlier: too small for the MacBook, but it is there already.
        assert_eq!(keep_on_desk(&["hd720", "hd720"], false), Some("hd720".to_string()));
        assert_eq!(keep_on_desk(&["sample", "sample"], true), Some("sample".to_string()));
    }

    #[test]
    fn leaving_each_mode_keeps_a_wallpaper_that_can_go_on_every_display() {
        // MacBook shows the panorama, the monitor the small sample: the panorama can go on both.
        assert_eq!(keep_on_desk(&["panorama", "sample"], false), Some("panorama".to_string()));
        // The first that can is taken, not simply the first.
        let monitor_first: Vec<DisplayInfo> = desk().into_iter().rev().collect();
        let sizes = library();
        let size = |id: &str| sizes.iter().find(|(known, _)| known == id).map(|(_, size)| *size);
        let kept = keep_for_all(
            &ids(&["sample", "fourk"]),
            |id, slot| size(id).is_some_and(|size| fit::fits_display(&size, &monitor_first[slot])),
            |_| false,
        );
        assert_eq!(kept, Some("fourk".to_string()));
    }

    #[test]
    fn a_wallpaper_is_never_spread_onto_a_display_it_is_too_small_for() {
        // Only the monitor shows something, and it is too small for the MacBook.
        assert_eq!(keep_on_desk(&["", "sample"], false), None);
        assert_eq!(keep_on_desk(&["", "sample"], true), None);
        // Each display shows a picture that is too small for the other one.
        assert_eq!(keep_on_desk(&["hd720", "sample"], false), None);
        // Nothing is showing at all.
        assert_eq!(keep_on_desk(&["", ""], false), None);
    }

    #[test]
    fn a_newly_connected_display_gets_the_wallpaper_only_if_it_is_large_enough() {
        // The monitor shows the sample; the MacBook has just been connected.
        let ordered: Vec<String> = desk().iter().map(|display| display.id.clone()).collect();
        let view = per_display_view(&ids(&["sample"]), &Layout::Same, &ordered, &ids(&["monitor"]));
        assert_eq!(view, ids(&["", "sample"]));
        assert_eq!(keep_on_desk(&["", "sample"], false), None, "too small for the MacBook: not put there");
        // The other way round, the monitor is connected to a MacBook showing the 4K picture.
        let view = per_display_view(&ids(&["fourk"]), &Layout::Same, &ordered, &ids(&["mac"]));
        assert_eq!(view, ids(&["fourk", ""]));
        assert_eq!(keep_on_desk(&["fourk", ""], false), Some("fourk".to_string()));
    }

    #[test]
    fn a_wallpaper_that_can_be_stretched_may_cover_a_new_display() {
        // Not on the monitor yet, but it has the pixels to span both.
        assert_eq!(keep_on_desk(&["panorama", ""], true), Some("panorama".to_string()));
        // The 4K picture cannot span the desk, yet it fits each display on its own.
        assert_eq!(keep_on_desk(&["fourk", ""], true), Some("fourk".to_string()));
        assert_eq!(single_layout(true, Some(&sized(3840.0, 2400.0)), &desk()), Layout::Same);
    }

    #[test]
    fn when_nothing_can_be_kept_the_fallback_is_an_automatic_change_or_nothing() {
        // What the worker does next for ["", sample] going to "same": one automatic change...
        let showing = EngineState { current: ids(&["", "sample"]), ..Default::default() };
        let mut random = rng(3);
        let (next, layout) = next_frame(&showing, DisplayMode::Same, &library(), &desk(), false, &mut random).unwrap();
        assert_eq!((next.current.clone(), layout), (ids(&["panorama"]), Layout::Same));
        assert_eq!(next.history, vec![ids(&["", "sample"])], "recorded like any other change");
        // ...and with only small pictures in the plan there is none, so the frame stays as it is.
        assert!(next_frame(&showing, DisplayMode::Same, &only(&["sample", "hd720"]), &desk(), false, &mut random).is_none());
        let left_alone = Saved {
            engine: showing,
            layout: Layout::PerDisplay(ids(&["mac", "monitor"])),
            ..Default::default()
        };
        assert_eq!(
            status_of(&left_alone).current,
            vec![CurrentEntry { display_id: Some("monitor".into()), item_id: "sample".into() }]
        );
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
        let view = per_display_view(&before.engine.current, &before.layout, &ordered, &ordered);
        assert_eq!(view, ids(&["a", "b"]));
        let staged = EngineState { current: view, ..before.engine.clone() };
        let slot = ordered.iter().position(|id| id == "right").unwrap();
        let after = Saved {
            engine: engine::show_in_slot(&staged, slot, "c"),
            layout: Layout::PerDisplay(ordered.clone()),
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
        let targets = plan_targets(&only, &[Some(image("c"))], &two_displays());
        assert_eq!(targets.len(), 1);
        assert_eq!((targets[0].display.id.as_str(), targets[0].item.id.as_str()), ("right", "c"));
    }

    #[test]
    fn applying_to_one_display_claims_nothing_about_the_others() {
        let ordered = ids(&["left", "right"]);
        // Still has shown nothing yet.
        let view = per_display_view(&[], &Layout::Same, &ordered, &ordered);
        let staged = EngineState { current: view, ..Default::default() };
        let after = Saved {
            engine: engine::show_in_slot(&staged, 1, "c"),
            layout: Layout::PerDisplay(ordered),
            ..Default::default()
        };
        assert_eq!(after.engine.current, ids(&["", "c"]));
        assert!(after.engine.history.is_empty(), "nothing was showing, so nothing to go back to");
        assert_eq!(
            status_of(&after).current,
            vec![CurrentEntry { display_id: Some("right".into()), item_id: "c".into() }]
        );
    }

    #[test]
    fn a_removed_wallpaper_leaves_a_gap_and_the_layout_stays_lined_up() {
        let mut state = saved(&["a", "gone", "c"], Layout::PerDisplay(ids(&["d1", "d2", "d3"])));
        state.engine.history = vec![ids(&["gone"]), ids(&["a", "gone"])];
        state.engine = engine::reconcile(&state.engine, &ids(&["a", "c"]));
        assert_eq!(state.engine.current, ids(&["a", "", "c"]));
        assert_eq!(state.engine.history, vec![ids(&["a", ""])]);
        assert_eq!(
            status_of(&state).current,
            vec![
                CurrentEntry { display_id: Some("d1".into()), item_id: "a".into() },
                CurrentEntry { display_id: Some("d3".into()), item_id: "c".into() },
            ]
        );
    }

    // --- size rules, as the worker combines them -----------------------------

    #[test]
    fn a_wallpaper_is_stretched_only_when_asked_and_able() {
        let panorama = sized(7680.0, 2400.0);
        let four_k = sized(3840.0, 2400.0);
        assert_eq!(single_layout(true, Some(&panorama), &desk()), Layout::Span);
        assert_eq!(single_layout(true, Some(&four_k), &desk()), Layout::Same);
        assert_eq!(single_layout(false, Some(&panorama), &desk()), Layout::Same);
        // One display cannot be spanned, and an unknown wallpaper never is.
        assert_eq!(single_layout(true, Some(&panorama), &desk()[..1]), Layout::Same);
        assert_eq!(single_layout(true, None, &desk()), Layout::Same);
        // A picture whose size nobody knows, and a scene, may be stretched.
        assert_eq!(single_layout(true, Some(&sized(0.0, 0.0)), &desk()), Layout::Span);
        let scene = FitItem { kind: WallpaperKind::Scene, width: 0.0, height: 0.0, per_display: false };
        assert_eq!(single_layout(true, Some(&scene), &desk()), Layout::Span);
    }

    #[test]
    fn automatic_rotation_leaves_out_what_is_too_small_for_any_display() {
        assert_eq!(eligible_ids(&library(), &desk(), DisplayMode::Same), ids(&["panorama", "fourk"]));
        // Span uses the same wallpapers; whether each is stretched is decided per wallpaper.
        assert_eq!(eligible_ids(&library(), &desk(), DisplayMode::Span), ids(&["panorama", "fourk"]));
        // On the monitor alone the sample is fine.
        assert_eq!(eligible_ids(&library(), &desk()[1..], DisplayMode::Same), ids(&["panorama", "fourk", "sample"]));
        assert!(eligible_ids(&only(&["sample", "hd720"]), &desk(), DisplayMode::Span).is_empty());
    }

    #[test]
    fn each_display_is_offered_only_what_is_large_enough_for_it() {
        let allowed = allowed_per_display(&library(), &desk());
        let set = |names: &[&str]| -> HashSet<String> { names.iter().map(|name| name.to_string()).collect() };
        assert_eq!(allowed, vec![set(&["panorama", "fourk"]), set(&["panorama", "fourk", "sample"])]);
    }

    #[test]
    fn same_mode_never_shows_the_small_sample_on_the_retina_desk() {
        for shuffle in [true, false] {
            let mut random = rng(4);
            let mut state = EngineState::default();
            for _ in 0..12 {
                let (next, layout) =
                    next_frame(&state, DisplayMode::Same, &library(), &desk(), shuffle, &mut random).unwrap();
                assert_eq!(layout, Layout::Same);
                assert!(["panorama", "fourk"].contains(&next.current[0].as_str()), "got {}", next.current[0]);
                state = next;
            }
        }
    }

    #[test]
    fn span_mode_stretches_what_it_can_and_shows_the_rest_on_each_display() {
        let mut random = rng(0);
        let mut state = EngineState::default();
        let mut seen = Vec::new();
        for _ in 0..4 {
            let (next, layout) = next_frame(&state, DisplayMode::Span, &library(), &desk(), false, &mut random).unwrap();
            seen.push((next.current[0].clone(), layout));
            state = next;
        }
        assert_eq!(
            seen,
            vec![
                ("panorama".to_string(), Layout::Span),
                ("fourk".to_string(), Layout::Same),
                ("panorama".to_string(), Layout::Span),
                ("fourk".to_string(), Layout::Same),
            ]
        );
        // The status is one entry without a display either way.
        let shown_per_display = Saved { engine: state, layout: Layout::Same, ..Default::default() };
        assert_eq!(status_of(&shown_per_display).current, vec![CurrentEntry { display_id: None, item_id: "fourk".into() }]);
    }

    #[test]
    fn with_one_display_every_mode_shows_one_wallpaper_on_it() {
        let monitor = &desk()[1..];
        let mut random = rng(0);
        let (same, layout) =
            next_frame(&EngineState::default(), DisplayMode::Same, &library(), monitor, false, &mut random).unwrap();
        assert_eq!((same.current, layout), (ids(&["panorama"]), Layout::Same));
        let (span, layout) =
            next_frame(&EngineState::default(), DisplayMode::Span, &library(), monitor, false, &mut random).unwrap();
        assert_eq!((span.current, layout), (ids(&["panorama"]), Layout::Same));
        let (each, layout) =
            next_frame(&EngineState::default(), DisplayMode::Each, &library(), monitor, false, &mut random).unwrap();
        assert_eq!((each.current, layout), (ids(&["panorama"]), Layout::PerDisplay(ids(&["monitor"]))));
    }

    #[test]
    fn each_mode_gives_every_display_something_large_enough() {
        for shuffle in [true, false] {
            let mut random = rng(6);
            let mut state = EngineState::default();
            for _ in 0..20 {
                let (next, layout) =
                    next_frame(&state, DisplayMode::Each, &library(), &desk(), shuffle, &mut random).unwrap();
                assert_eq!(layout, Layout::PerDisplay(ids(&["mac", "monitor"])));
                assert!(["panorama", "fourk"].contains(&next.current[0].as_str()), "mac got {}", next.current[0]);
                assert!(["panorama", "fourk", "sample"].contains(&next.current[1].as_str()));
                assert_ne!(next.current[0], next.current[1]);
                state = next;
            }
        }
    }

    /// What the owner hit: only 1672 x 941 pictures, a Retina MacBook and a 1080p monitor.
    #[test]
    fn a_library_of_small_pictures_never_reaches_the_retina_display() {
        let small = only(&["sample"]);
        let mut random = rng(1);
        let showing = EngineState { current: ids(&["earlier"]), ..Default::default() };
        // Same and span: nothing is large enough, so nothing changes.
        assert!(next_frame(&showing, DisplayMode::Same, &small, &desk(), true, &mut random).is_none());
        assert!(next_frame(&showing, DisplayMode::Span, &small, &desk(), true, &mut random).is_none());
        // Each: the monitor gets it, the MacBook is left exactly as it is.
        let (next, layout) = next_frame(&showing, DisplayMode::Each, &small, &desk(), true, &mut random).unwrap();
        assert_eq!(next.current, ids(&["", "sample"]));
        let targets = plan_targets(&layout, &[None, Some(image("sample"))], &desk());
        assert_eq!(targets.len(), 1);
        assert_eq!(targets[0].display.id, "monitor");
        let status = status_of(&Saved { engine: next, layout, ..Default::default() });
        assert_eq!(status.current, vec![CurrentEntry { display_id: Some("monitor".into()), item_id: "sample".into() }]);
        // Nothing usable on either display: nothing happens in any mode.
        assert!(next_frame(&showing, DisplayMode::Each, &only(&["hd720"]), &desk(), true, &mut random).is_none());
        assert!(next_frame(&showing, DisplayMode::Each, &[], &desk(), true, &mut random).is_none());
    }

    #[test]
    fn switching_to_each_keeps_the_first_display_and_fills_the_rest_by_size() {
        // "fourk" is on both displays; the plan now gives each display its own.
        let state = EngineState { current: ids(&["fourk"]), ..Default::default() };
        let ordered: Vec<String> = desk().iter().map(|display| display.id.clone()).collect();
        let kept = per_display_view(&state.current, &Layout::Same, &ordered, &ordered[..1]);
        assert_eq!(kept, ids(&["fourk", ""]));
        let candidates = only(&["fourk", "sample"]);
        let plan_ids: Vec<String> = candidates.iter().map(|(id, _)| id.clone()).collect();
        let allowed = allowed_per_display(&candidates, &desk());
        let mut random = rng(2);
        let filled = engine::fill_slots(&state, &kept, &plan_ids, &allowed, false, &mut random);
        assert_eq!(filled.current, ids(&["fourk", "sample"]));
        assert!(filled.history.is_empty(), "re-laying out is not a change");

        // The other way round the MacBook has nothing large enough: it gets a
        // gap, which means it is left showing what it shows.
        let swapped: Vec<DisplayInfo> = desk().into_iter().rev().collect();
        let ordered: Vec<String> = swapped.iter().map(|display| display.id.clone()).collect();
        let state = EngineState { current: ids(&["sample"]), ..Default::default() };
        let kept = per_display_view(&state.current, &Layout::Same, &ordered, &ordered[..1]);
        let allowed = allowed_per_display(&only(&["sample"]), &swapped);
        let filled = engine::fill_slots(&state, &kept, &ids(&["sample"]), &allowed, false, &mut random);
        assert_eq!(filled.current, ids(&["sample", ""]));
    }

    // --- scenes that draw themselves whole on each display ---------------------

    #[test]
    fn the_size_rules_see_what_the_item_says_about_itself() {
        let clock = fit_item(&scene_item("clock", true), None);
        assert!(clock.per_display);
        assert_eq!((clock.kind, clock.width, clock.height), (WallpaperKind::Scene, 1920.0, 1200.0));
        assert!(!fit_item(&scene_item("aurora", false), None).per_display);
        // The size the UI sent wins; a size read from the file fills in when there is none.
        let known = PlayItem { width: 3840.0, height: 2400.0, ..image("a") };
        assert_eq!(fit_item(&known, Some((10, 10))).width, 3840.0);
        let measured = fit_item(&image("a"), Some((1672, 941)));
        assert_eq!((measured.width, measured.height), (1672.0, 941.0));
        assert_eq!(fit_item(&image("a"), None).width, 0.0);
    }

    /// The span-mode turn as the worker takes it: the next frame, its layout,
    /// and what each display is then asked to show.
    #[test]
    fn in_span_mode_a_per_display_scene_takes_its_turn_whole_on_each_display() {
        let clock = scene_item("clock", true);
        let aurora = scene_item("aurora", false);
        let candidates = vec![
            ("clock".to_string(), fit_item(&clock, None)),
            ("aurora".to_string(), fit_item(&aurora, None)),
        ];
        let mut random = rng(0);

        let (first, layout) =
            next_frame(&EngineState::default(), DisplayMode::Span, &candidates, &desk(), false, &mut random).unwrap();
        assert_eq!(first.current, ids(&["clock"]));
        assert_eq!(layout, Layout::Same, "recorded as what is on screen: the same scene on each display");
        let targets = plan_targets(&layout, &[Some(clock.clone())], &desk());
        assert_eq!(targets.len(), 2);
        for target in &targets {
            // Each window is its own canvas, and the poster underneath is set
            // whole per display rather than cut into span pieces.
            assert!(!target.spanned);
            assert_eq!((target.viewport.x, target.viewport.y), (0.0, 0.0));
            assert_eq!(
                (target.viewport.canvas_width, target.viewport.canvas_height),
                (target.display.width, target.display.height)
            );
            assert_eq!(target.item.poster_path.as_deref(), Some("/lib/clock.jpg"));
        }
        let showing = Saved { engine: first.clone(), layout, ..Default::default() };
        assert_eq!(status_of(&showing).current, vec![CurrentEntry { display_id: None, item_id: "clock".into() }]);

        // The ordinary scene's turn: one picture across both displays.
        let (second, layout) = next_frame(&first, DisplayMode::Span, &candidates, &desk(), false, &mut random).unwrap();
        assert_eq!(second.current, ids(&["aurora"]));
        assert_eq!(layout, Layout::Span);
        let targets = plan_targets(&layout, &[Some(aurora)], &desk());
        assert!(targets.iter().all(|target| target.spanned));
        assert_eq!((targets[0].viewport.canvas_width, targets[0].viewport.canvas_height), (3648.0, 1117.0));
        assert_eq!((targets[1].viewport.x, targets[1].viewport.y), (1728.0, 0.0));
    }

    #[test]
    fn a_per_display_scene_is_never_recorded_as_spanned() {
        let clock = fit_item(&scene_item("clock", true), None);
        // Asked for by hand as "one across all": laid out on each display instead.
        assert_eq!(single_layout(true, Some(&clock), &desk()), Layout::Same);
        // Showing on both displays when the mode changes to span: it is kept, as "same".
        let kept = keep_for_all(
            &ids(&["clock", "clock"]),
            |_, slot| fit::fits_display(&clock, &desk()[slot]),
            |_| fit::can_span(&clock, &desk()),
        );
        assert_eq!(kept, Some("clock".to_string()));
        assert_eq!(single_layout(true, Some(&clock), &desk()), Layout::Same);
        // It is still eligible in every mode: a scene fits any display.
        let candidates = vec![("clock".to_string(), clock)];
        assert_eq!(eligible_ids(&candidates, &desk(), DisplayMode::Span), ids(&["clock"]));
        assert_eq!(eligible_ids(&candidates, &desk(), DisplayMode::Same), ids(&["clock"]));
        // An ordinary scene is still one picture across the displays.
        let aurora = fit_item(&scene_item("aurora", false), None);
        assert_eq!(single_layout(true, Some(&aurora), &desk()), Layout::Span);
    }

    // --- saved state ---------------------------------------------------------

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
            engine: EngineState { bag: ids(&["a"]), current: ids(&["", "b"]), history: vec![ids(&["c"])] },
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

    /// A `rotation.json` exactly as the previous build wrote it: items without
    /// a size, frames without gaps. It must load as it is.
    #[test]
    fn state_written_by_the_previous_build_loads_unchanged() {
        let on_disk = r#"{
          "plan": {
            "items": [
              { "id": "glider", "kind": "image", "path": "/lib/glider.jpg" },
              { "id": "clip", "kind": "video", "path": "/lib/clip.mp4", "posterPath": "/lib/clip.jpg" }
            ],
            "shuffle": true,
            "schedule": { "type": "interval", "minutes": 60.0 },
            "paused": false,
            "displayMode": "each",
            "pauseLiveOnBattery": true
          },
          "paused": false,
          "engine": { "bag": ["clip"], "current": ["glider", "clip"], "history": [["clip"], ["glider", "glider"]] },
          "layout": { "kind": "perDisplay", "displays": ["1", "2"] },
          "nextChangeAt": 1794303000000,
          "extras": [{ "id": "wing", "kind": "image", "path": "/lib/wing.jpg" }]
        }"#;
        let state: Saved = serde_json::from_str(on_disk).expect("old state should load");
        let plan = state.plan.as_ref().unwrap();
        assert_eq!(plan.items.len(), 2);
        assert_eq!((plan.items[0].width, plan.items[0].height), (0.0, 0.0), "no size means unknown");
        assert!(plan.items.iter().all(|item| !item.per_display), "no flag means an ordinary wallpaper");
        assert_eq!(plan.items[1].poster_path.as_deref(), Some("/lib/clip.jpg"));
        assert_eq!(plan.display_mode, DisplayMode::Each);
        assert_eq!(state.engine.bag, ids(&["clip"]));
        assert_eq!(state.engine.current, ids(&["glider", "clip"]));
        assert_eq!(state.engine.history, vec![ids(&["clip"]), ids(&["glider", "glider"])]);
        assert_eq!(state.layout, Layout::PerDisplay(ids(&["1", "2"])));
        assert_eq!(state.next_change_at, Some(1_794_303_000_000));
        assert_eq!(state.extras[0].id, "wing");
        assert_eq!(status_of(&state).current.len(), 2);
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
