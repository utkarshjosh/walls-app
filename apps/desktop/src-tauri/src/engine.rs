//! Which wallpaper comes next. Pure: no clock, no files, no AppKit.
//!
//! A faithful port of `apps/web/src/lib/rotation.ts`, which documents the
//! rules both sides must follow:
//!
//!  - A "frame" is what is on the desktop: one wallpaper id normally, or one
//!    per display when each display shows something different. Per-display
//!    frames are positional (entry i belongs to display i); "" means the app
//!    has put nothing on that display.
//!  - Callers pass only the wallpapers that are large enough for the displays
//!    (see `fit.rs`); the rules here never see the ones that are too small.
//!  - Shuffle draws from a bag holding every wallpaper once, so nothing repeats
//!    until all have been shown. A fresh bag never starts with what is showing.
//!  - In order walks the list and wraps.
//!  - Previous steps back through what was actually shown, shuffled or not.
//!
//! `fill_slots`, `show_in_slot` and `has_any` have no TypeScript counterpart:
//! they serve re-layouts that only the native side performs.

use std::collections::HashSet;

use serde::{Deserialize, Serialize};

pub const HISTORY_LIMIT: usize = 50;

#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(default)]
pub struct EngineState {
    /// Ids still to be drawn in shuffle mode; the next one is at the front.
    pub bag: Vec<String>,
    /// What is showing now. Empty until the app first sets a wallpaper.
    pub current: Vec<String>,
    /// Earlier frames, most recent last.
    pub history: Vec<Vec<String>>,
}

/// True when the frame shows at least one wallpaper. A frame of only ""
/// entries says nothing and is treated like an empty one everywhere.
pub fn has_any(frame: &[String]) -> bool {
    frame.iter().any(|id| !id.is_empty())
}

/// After the library changes: forget wallpapers that are gone, keep everything
/// else. A frame keeps its positions (a gone wallpaper becomes ""), and a frame
/// with nothing left in it is dropped.
pub fn reconcile(state: &EngineState, ids: &[String]) -> EngineState {
    let keep = |frame: &Vec<String>| -> Vec<String> {
        let kept: Vec<String> =
            frame.iter().map(|id| if ids.contains(id) { id.clone() } else { String::new() }).collect();
        if has_any(&kept) {
            kept
        } else {
            Vec::new()
        }
    };
    EngineState {
        bag: state.bag.iter().filter(|id| ids.contains(id)).cloned().collect(),
        current: keep(&state.current),
        history: state.history.iter().map(keep).filter(|frame| !frame.is_empty()).collect(),
    }
}

/// Move to the next frame of `count` wallpapers. Returns the state unchanged
/// when there is nothing to show. `random` yields numbers in `[0, 1)`.
pub fn advance(
    state: &EngineState,
    ids: &[String],
    shuffle: bool,
    count: usize,
    random: &mut dyn FnMut() -> f64,
) -> EngineState {
    let ids = unique(ids);
    if ids.is_empty() {
        return state.clone();
    }
    let count = count.max(1);
    let last_shown = state.current.last().cloned();
    let (bag, frame) = if shuffle {
        draw_shuffled(sanitized_bag(&state.bag, &ids, &[]), Vec::new(), last_shown, &ids, count, random)
    } else {
        (Vec::new(), draw_in_order(Vec::new(), last_shown, &ids, count))
    };
    EngineState { bag, current: frame, history: push_history(state) }
}

/// Next frame for "different on each display", where a wallpaper may be large
/// enough for some displays and not others.
///
/// `allowed[i]` holds the ids display i may show. The result has one entry per
/// display; "" means nothing is large enough for that display, so it is left
/// as it is. Displays get different wallpapers whenever there is a choice.
/// With every id allowed everywhere this behaves like `advance` with a count.
pub fn advance_slots(
    state: &EngineState,
    ids: &[String],
    allowed: &[HashSet<String>],
    shuffle: bool,
    random: &mut dyn FnMut() -> f64,
) -> EngineState {
    let blank = vec![String::new(); allowed.len()];
    match draw_slots(state, &blank, &state.current, ids, allowed, shuffle, random) {
        Some((bag, frame)) => EngineState { bag, current: frame, history: push_history(state) },
        None => state.clone(),
    }
}

/// Complete a per-display frame without counting it as a change (no history
/// entry): entries of `kept` that name a wallpaper stay, and each "" is drawn
/// the way `advance_slots` would draw it, or stays "" when nothing is allowed
/// on that display. Used when the display mode becomes "each" or a display is
/// connected.
pub fn fill_slots(
    state: &EngineState,
    kept: &[String],
    ids: &[String],
    allowed: &[HashSet<String>],
    shuffle: bool,
    random: &mut dyn FnMut() -> f64,
) -> EngineState {
    let nothing_to_draw = kept.len() >= allowed.len() && kept.iter().all(|id| !id.is_empty());
    let drawn = if nothing_to_draw { None } else { draw_slots(state, kept, kept, ids, allowed, shuffle, random) };
    match drawn {
        Some((bag, frame)) => EngineState { bag, current: frame, history: state.history.clone() },
        None => EngineState { current: kept.to_vec(), ..state.clone() },
    }
}

/// Step back to the previously shown frame, or `None` when there is no history.
pub fn back(state: &EngineState) -> Option<EngineState> {
    let mut history = state.history.clone();
    let current = history.pop()?;
    Some(EngineState { bag: state.bag.clone(), current, history })
}

/// The person picked a wallpaper by hand: show it and remember what it replaced.
pub fn show(state: &EngineState, frame: &[String]) -> EngineState {
    if !has_any(frame) {
        return state.clone();
    }
    EngineState {
        bag: state.bag.iter().filter(|id| !frame.contains(id)).cloned().collect(),
        current: frame.to_vec(),
        history: push_history(state),
    }
}

/// The person put one wallpaper on one display only: replace that display's
/// entry in a per-display frame, leave the other entries alone, and remember
/// the whole frame that was showing. `slot` is the display's position in the
/// frame; an out-of-range slot changes nothing.
pub fn show_in_slot(state: &EngineState, slot: usize, id: &str) -> EngineState {
    if slot >= state.current.len() {
        return state.clone();
    }
    let mut frame = state.current.clone();
    frame[slot] = id.to_string();
    EngineState {
        bag: state.bag.iter().filter(|queued| queued.as_str() != id).cloned().collect(),
        current: frame,
        history: push_history(state),
    }
}

fn push_history(state: &EngineState) -> Vec<Vec<String>> {
    let mut history = state.history.clone();
    if !has_any(&state.current) {
        return history;
    }
    history.push(state.current.clone());
    if history.len() > HISTORY_LIMIT {
        let surplus = history.len() - HISTORY_LIMIT;
        history.drain(..surplus);
    }
    history
}

fn draw_shuffled(
    mut bag: Vec<String>,
    mut frame: Vec<String>,
    last_shown: Option<String>,
    ids: &[String],
    count: usize,
    random: &mut dyn FnMut() -> f64,
) -> (Vec<String>, Vec<String>) {
    while frame.len() < count {
        if bag.is_empty() {
            bag = shuffled(ids, random);
            // Do not open a new round with the wallpaper that is on screen.
            let avoid = frame.last().or(last_shown.as_ref());
            if bag.len() > 1 && Some(&bag[0]) == avoid {
                bag.rotate_left(1);
            }
        }
        let next = bag.remove(0);
        // Different displays should not show the same picture when there is a choice.
        // `ids` is duplicate-free, so with `ids.len() >= count` some id is always
        // still missing from the frame and this cannot spin forever.
        if frame.contains(&next) && ids.len() >= count {
            bag.push(next);
            continue;
        }
        frame.push(next);
    }
    (bag, frame)
}

fn draw_in_order(
    mut frame: Vec<String>,
    last_shown: Option<String>,
    ids: &[String],
    count: usize,
) -> Vec<String> {
    // An id that is no longer in the list restarts from the top, like indexOf() === -1.
    let mut index = match last_shown.and_then(|last| ids.iter().position(|id| *id == last)) {
        Some(position) => position + 1,
        None => 0,
    };
    while frame.len() < count {
        index %= ids.len();
        frame.push(ids[index].clone());
        index += 1;
    }
    frame
}

/// The draw behind `advance_slots` and `fill_slots`: one entry per display.
///
/// Slots for which `kept` already names a wallpaper keep it; the others are
/// drawn from what their display allows. `reference` is the frame "in order"
/// continues after. `None` when no id is allowed anywhere (or there are no
/// displays), which callers treat as "nothing to do".
fn draw_slots(
    state: &EngineState,
    kept: &[String],
    reference: &[String],
    ids: &[String],
    allowed: &[HashSet<String>],
    shuffle: bool,
    random: &mut dyn FnMut() -> f64,
) -> Option<(Vec<String>, Vec<String>)> {
    let ids = unique(ids);
    let usable: Vec<String> =
        ids.iter().filter(|id| allowed.iter().any(|set| set.contains(*id))).cloned().collect();
    if usable.is_empty() || allowed.is_empty() {
        return None;
    }
    let mut bag = if shuffle { sanitized_bag(&state.bag, &usable, &[]) } else { Vec::new() };
    // In order: one pointer walks the list, starting after the last wallpaper shown.
    let mut pointer: Option<usize> = reference
        .iter()
        .rev()
        .find(|id| !id.is_empty())
        .and_then(|last| ids.iter().position(|id| id == last));
    let mut frame: Vec<String> = Vec::with_capacity(allowed.len());

    for (slot, allowed_here) in allowed.iter().enumerate() {
        if let Some(keep) = kept.get(slot).filter(|id| !id.is_empty()) {
            frame.push(keep.clone());
            continue;
        }
        let fits: Vec<&String> = usable.iter().filter(|id| allowed_here.contains(*id)).collect();
        if fits.is_empty() {
            frame.push(String::new());
            continue;
        }
        // Repeat a wallpaper on a second display only when this display has no other choice.
        let taken = |id: &String| frame.contains(id) || kept.iter().skip(slot + 1).any(|later| later == id);
        let unused: Vec<&String> = fits.iter().copied().filter(|id| !taken(id)).collect();
        let pool = if unused.is_empty() { fits } else { unused };
        let mut pick = pool[0].clone();

        if shuffle {
            let mut index = bag.iter().position(|id| pool.contains(&id));
            if index.is_none() {
                // This round has nothing left for this display: deal a new one behind it,
                // and do not open with what the display is already showing.
                for id in shuffled(&usable, random) {
                    if !bag.contains(&id) {
                        bag.push(id);
                    }
                }
                let showing = state.current.get(slot);
                index = bag
                    .iter()
                    .position(|id| pool.contains(&id) && (pool.len() == 1 || Some(id) != showing))
                    .or_else(|| bag.iter().position(|id| pool.contains(&id)));
            }
            if let Some(index) = index {
                pick = bag.remove(index);
            }
        } else {
            // A display moves on from what it is showing whenever it has another
            // choice; otherwise the shared pointer could land every display back on
            // the same wallpapers and "Next" would appear to do nothing.
            let showing = state.current.get(slot);
            let choices: Vec<&String> =
                if pool.len() > 1 { pool.iter().copied().filter(|id| Some(*id) != showing).collect() } else { pool };
            if let Some(first) = choices.first() {
                pick = (*first).clone();
            }
            let after = pointer.map_or(0, |index| index + 1);
            for step in 0..ids.len() {
                let index = (after + step) % ids.len();
                if choices.contains(&&ids[index]) {
                    pick = ids[index].clone();
                    pointer = Some(index);
                    break;
                }
            }
        }
        frame.push(pick);
    }
    Some((bag, frame))
}

fn shuffled(ids: &[String], random: &mut dyn FnMut() -> f64) -> Vec<String> {
    let mut out = ids.to_vec();
    for i in (1..out.len()).rev() {
        // Clamp: a generator that returns exactly 1.0 must not index past `i`.
        let j = ((random() * (i as f64 + 1.0)).floor() as usize).min(i);
        out.swap(i, j);
    }
    out
}

/// The saved bag, limited to ids that still exist, without repeats (a damaged
/// state file must not be able to break the draw), minus `exclude`.
fn sanitized_bag(bag: &[String], ids: &[String], exclude: &[String]) -> Vec<String> {
    let mut out: Vec<String> = Vec::with_capacity(bag.len());
    for id in bag {
        if ids.contains(id) && !exclude.contains(id) && !out.contains(id) {
            out.push(id.clone());
        }
    }
    out
}

fn unique(ids: &[String]) -> Vec<String> {
    let mut out: Vec<String> = Vec::with_capacity(ids.len());
    for id in ids {
        if !out.contains(id) {
            out.push(id.clone());
        }
    }
    out
}

/// Small self-contained random source (SplitMix64) so the engine needs no
/// dependency. Quality is ample for shuffling wallpapers.
pub struct Rng(u64);

impl Rng {
    pub fn from_seed(seed: u64) -> Self {
        Rng(seed)
    }

    /// Uniform in `[0, 1)`.
    pub fn next_f64(&mut self) -> f64 {
        self.0 = self.0.wrapping_add(0x9E37_79B9_7F4A_7C15);
        let mut z = self.0;
        z = (z ^ (z >> 30)).wrapping_mul(0xBF58_476D_1CE4_E5B9);
        z = (z ^ (z >> 27)).wrapping_mul(0x94D0_49BB_1331_11EB);
        z ^= z >> 31;
        // 53 random bits fill a double's mantissa exactly.
        (z >> 11) as f64 / (1u64 << 53) as f64
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn ids(names: &[&str]) -> Vec<String> {
        names.iter().map(|name| name.to_string()).collect()
    }

    fn frame(names: &[&str]) -> Vec<String> {
        ids(names)
    }

    fn rng(seed: u64) -> impl FnMut() -> f64 {
        let mut source = Rng::from_seed(seed);
        move || source.next_f64()
    }

    #[test]
    fn shuffle_shows_every_item_once_before_repeating() {
        let all = ids(&["a", "b", "c", "d", "e", "f", "g"]);
        for seed in 0..40 {
            let mut random = rng(seed);
            let mut state = EngineState::default();
            for _round in 0..4 {
                let mut seen = HashSet::new();
                for _ in 0..all.len() {
                    state = advance(&state, &all, true, 1, &mut random);
                    assert_eq!(state.current.len(), 1);
                    assert!(seen.insert(state.current[0].clone()), "repeat within a round, seed {seed}");
                }
                assert_eq!(seen.len(), all.len());
                assert!(state.bag.is_empty());
            }
        }
    }

    #[test]
    fn a_new_round_does_not_start_with_the_item_on_screen() {
        let all = ids(&["a", "b", "c"]);
        for seed in 0..200 {
            let mut random = rng(seed);
            let mut state = EngineState::default();
            let mut previous: Option<String> = None;
            for _ in 0..30 {
                state = advance(&state, &all, true, 1, &mut random);
                assert_ne!(Some(&state.current[0]), previous.as_ref(), "back-to-back repeat, seed {seed}");
                previous = Some(state.current[0].clone());
            }
        }
    }

    #[test]
    fn a_single_item_keeps_showing_in_shuffle() {
        let all = ids(&["only"]);
        let mut random = rng(1);
        let mut state = EngineState::default();
        for _ in 0..3 {
            state = advance(&state, &all, true, 1, &mut random);
            assert_eq!(state.current, frame(&["only"]));
        }
    }

    #[test]
    fn in_order_walks_the_list_and_wraps() {
        let all = ids(&["a", "b", "c"]);
        let mut random = rng(0);
        let mut state = EngineState::default();
        let mut shown = Vec::new();
        for _ in 0..7 {
            state = advance(&state, &all, false, 1, &mut random);
            shown.push(state.current[0].clone());
            assert!(state.bag.is_empty());
        }
        assert_eq!(shown, ids(&["a", "b", "c", "a", "b", "c", "a"]));
    }

    #[test]
    fn in_order_restarts_when_the_item_on_screen_left_the_list() {
        let mut random = rng(0);
        let state = EngineState { current: frame(&["gone"]), ..Default::default() };
        let next = advance(&state, &ids(&["a", "b"]), false, 1, &mut random);
        assert_eq!(next.current, frame(&["a"]));
    }

    #[test]
    fn in_order_frames_for_several_displays_continue_and_wrap() {
        let all = ids(&["a", "b", "c"]);
        let mut random = rng(0);
        let first = advance(&EngineState::default(), &all, false, 2, &mut random);
        assert_eq!(first.current, frame(&["a", "b"]));
        let second = advance(&first, &all, false, 2, &mut random);
        assert_eq!(second.current, frame(&["c", "a"]));
    }

    #[test]
    fn previous_returns_the_actual_prior_frames() {
        let all = ids(&["a", "b", "c", "d", "e"]);
        let mut random = rng(7);
        let mut state = EngineState::default();
        let mut shown = Vec::new();
        for _ in 0..4 {
            state = advance(&state, &all, true, 1, &mut random);
            shown.push(state.current.clone());
        }
        // Walk all the way back: each step lands on what was really shown before.
        for expected in shown.iter().rev().skip(1) {
            state = back(&state).expect("history should not be empty yet");
            assert_eq!(&state.current, expected);
        }
        assert!(back(&state).is_none(), "nothing before the first frame");
    }

    #[test]
    fn previous_keeps_the_shuffle_bag() {
        let all = ids(&["a", "b", "c", "d"]);
        let mut random = rng(3);
        let mut state = EngineState::default();
        state = advance(&state, &all, true, 1, &mut random);
        state = advance(&state, &all, true, 1, &mut random);
        let bag = state.bag.clone();
        let stepped = back(&state).unwrap();
        assert_eq!(stepped.bag, bag);
    }

    #[test]
    fn manual_show_records_history_and_leaves_the_bag_without_it() {
        let state = EngineState {
            bag: ids(&["c", "b", "d"]),
            current: frame(&["a"]),
            history: vec![],
        };
        let shown = show(&state, &frame(&["b"]));
        assert_eq!(shown.current, frame(&["b"]));
        assert_eq!(shown.history, vec![frame(&["a"])]);
        assert_eq!(shown.bag, ids(&["c", "d"]));
        assert_eq!(back(&shown).unwrap().current, frame(&["a"]));
        // Showing nothing is not a change.
        assert_eq!(show(&state, &[]), state);
    }

    #[test]
    fn the_first_frame_leaves_no_history() {
        let mut random = rng(0);
        let first = advance(&EngineState::default(), &ids(&["a", "b"]), false, 1, &mut random);
        assert!(first.history.is_empty());
        let shown = show(&EngineState::default(), &frame(&["a"]));
        assert!(shown.history.is_empty());
    }

    #[test]
    fn history_is_capped() {
        let all = ids(&["a", "b", "c"]);
        let mut random = rng(0);
        let mut state = EngineState::default();
        for _ in 0..(HISTORY_LIMIT + 25) {
            state = advance(&state, &all, false, 1, &mut random);
        }
        assert_eq!(state.history.len(), HISTORY_LIMIT);
        // The most recent entry is still the frame shown just before the current one.
        assert_ne!(state.history.last().unwrap(), &state.current);
    }

    #[test]
    fn reconcile_drops_removed_ids() {
        let state = EngineState {
            bag: ids(&["a", "x", "b"]),
            current: frame(&["x", "c"]),
            history: vec![frame(&["x"]), frame(&["a", "x"]), frame(&["b"])],
        };
        let kept = reconcile(&state, &ids(&["a", "b", "c"]));
        assert_eq!(kept.bag, ids(&["a", "b"]));
        // Frames keep their positions: the gone wallpaper leaves a gap.
        assert_eq!(kept.current, frame(&["", "c"]));
        // A frame that loses every id disappears; a partly surviving one keeps its shape.
        assert_eq!(kept.history, vec![frame(&["a", ""]), frame(&["b"])]);
    }

    #[test]
    fn advance_with_nothing_to_show_changes_nothing() {
        let state = EngineState { current: frame(&["a"]), ..Default::default() };
        let mut random = rng(0);
        assert_eq!(advance(&state, &[], true, 1, &mut random), state);
        assert_eq!(advance(&state, &[], false, 2, &mut random), state);
    }

    #[test]
    fn each_frames_hold_distinct_items_when_enough_exist() {
        let all = ids(&["a", "b", "c", "d", "e"]);
        for seed in 0..200 {
            let mut random = rng(seed);
            let mut state = EngineState::default();
            for _ in 0..20 {
                state = advance(&state, &all, true, 3, &mut random);
                let distinct: HashSet<_> = state.current.iter().collect();
                assert_eq!(state.current.len(), 3);
                assert_eq!(distinct.len(), 3, "duplicate inside a frame, seed {seed}");
            }
        }
    }

    #[test]
    fn each_frames_stay_distinct_when_items_equal_displays() {
        let all = ids(&["a", "b"]);
        for seed in 0..100 {
            let mut random = rng(seed);
            let mut state = EngineState::default();
            for _ in 0..10 {
                state = advance(&state, &all, true, 2, &mut random);
                assert_ne!(state.current[0], state.current[1], "seed {seed}");
            }
        }
    }

    #[test]
    fn each_frames_repeat_only_when_there_are_too_few_items() {
        let all = ids(&["a", "b"]);
        let mut random = rng(5);
        let state = advance(&EngineState::default(), &all, true, 3, &mut random);
        assert_eq!(state.current.len(), 3);
        assert!(state.current.iter().all(|id| all.contains(id)));
        let in_order = advance(&EngineState::default(), &all, false, 3, &mut random);
        assert_eq!(in_order.current, frame(&["a", "b", "a"]));
    }

    #[test]
    fn a_damaged_bag_or_duplicate_ids_cannot_hang_the_draw() {
        let state = EngineState {
            bag: ids(&["a", "a", "ghost", "a"]),
            current: frame(&["b"]),
            history: vec![],
        };
        let mut random = rng(9);
        let next = advance(&state, &ids(&["a", "b", "a", "b"]), true, 2, &mut random);
        let distinct: HashSet<_> = next.current.iter().collect();
        assert_eq!(distinct.len(), 2);
    }

    #[test]
    fn one_display_can_be_changed_on_its_own() {
        // Two displays showing [a, b]; the person puts c on the second one.
        let state = EngineState {
            bag: ids(&["d", "c", "e"]),
            current: frame(&["a", "b"]),
            history: vec![frame(&["z", "y"])],
        };
        let shown = show_in_slot(&state, 1, "c");
        assert_eq!(shown.current, frame(&["a", "c"]));
        assert_eq!(shown.history, vec![frame(&["z", "y"]), frame(&["a", "b"])]);
        // The hand-picked wallpaper will not come up again in this shuffle round.
        assert_eq!(shown.bag, ids(&["d", "e"]));
        // "Previous" puts the second display back to what it showed.
        assert_eq!(back(&shown).unwrap().current, frame(&["a", "b"]));

        let first = show_in_slot(&state, 0, "c");
        assert_eq!(first.current, frame(&["c", "b"]));
        // A slot that does not exist changes nothing.
        assert_eq!(show_in_slot(&state, 2, "c"), state);
    }

    fn sets(slots: &[&[&str]]) -> Vec<HashSet<String>> {
        slots.iter().map(|names| names.iter().map(|name| name.to_string()).collect()).collect()
    }

    #[test]
    fn blank_frames_count_as_nothing_showing() {
        assert!(!has_any(&[]));
        assert!(!has_any(&frame(&["", ""])));
        assert!(has_any(&frame(&["", "a"])));
        // Nothing was really showing, so nothing goes into history...
        let blank = EngineState { current: frame(&["", ""]), ..Default::default() };
        let mut random = rng(1);
        assert!(advance(&blank, &ids(&["a", "b"]), false, 1, &mut random).history.is_empty());
        assert!(show(&blank, &frame(&["a"])).history.is_empty());
        // ...and "showing" a blank frame is not a change.
        let state = EngineState { current: frame(&["a"]), ..Default::default() };
        assert_eq!(show(&state, &frame(&["", ""])), state);
    }

    #[test]
    fn filling_keeps_what_is_showing_and_draws_only_the_gaps() {
        let all = ids(&["a", "b", "c", "d"]);
        let everywhere = sets(&[&["a", "b", "c", "d"], &["a", "b", "c", "d"], &["a", "b", "c", "d"]]);
        for seed in 0..100 {
            let mut random = rng(seed);
            // "b" is on the first display and, as after "previous", still in the bag.
            let state = EngineState { bag: ids(&["b"]), current: frame(&["b"]), history: vec![frame(&["a"])] };
            let filled = fill_slots(&state, &frame(&["b", "", ""]), &all, &everywhere, true, &mut random);
            assert_eq!(filled.current.len(), 3);
            assert_eq!(filled.current[0], "b", "what is showing stays where it is");
            let distinct: HashSet<_> = filled.current.iter().collect();
            assert_eq!(distinct.len(), 3, "seed {seed}");
            assert_eq!(filled.history, state.history, "filling is not a change");
            assert!(!filled.bag.contains(&filled.current[1]) && !filled.bag.contains(&filled.current[2]));
        }
    }

    #[test]
    fn filling_in_order_continues_after_what_is_showing() {
        let all = ids(&["a", "b", "c"]);
        let everywhere = sets(&[&["a", "b", "c"], &["a", "b", "c"], &["a", "b", "c"]]);
        let mut random = rng(0);
        let state = EngineState { current: frame(&["b"]), ..Default::default() };
        let filled = fill_slots(&state, &frame(&["b", "", ""]), &all, &everywhere, false, &mut random);
        assert_eq!(filled.current, frame(&["b", "c", "a"]));
        // A gap in the middle avoids what the later display already shows.
        let middle = fill_slots(&state, &frame(&["a", "", "b"]), &all, &everywhere, false, &mut random);
        assert_eq!(middle.current, frame(&["a", "c", "b"]));
    }

    #[test]
    fn filling_respects_what_each_display_allows() {
        let all = ids(&["a", "b", "c"]);
        // The middle display is demanding: nothing is large enough for it.
        let allowed = sets(&[&["a", "b", "c"], &[], &["c"]]);
        for shuffle in [true, false] {
            let mut random = rng(3);
            let state = EngineState { current: frame(&["a"]), ..Default::default() };
            let filled = fill_slots(&state, &frame(&["a", "", ""]), &all, &allowed, shuffle, &mut random);
            assert_eq!(filled.current, frame(&["a", "", "c"]));
        }
        // What is showing stays even where it would no longer be allowed.
        let mut random = rng(3);
        let state = EngineState { current: frame(&["b"]), ..Default::default() };
        let strict = sets(&[&["c"], &["c"]]);
        let kept = fill_slots(&state, &frame(&["b", ""]), &all, &strict, false, &mut random);
        assert_eq!(kept.current, frame(&["b", "c"]));
    }

    #[test]
    fn filling_with_nothing_to_draw_changes_nothing_else() {
        let mut random = rng(0);
        let state = EngineState { bag: ids(&["x", "c"]), current: frame(&["a"]), history: vec![frame(&["z"])] };
        // A complete frame is taken as it is, bag untouched.
        let full = fill_slots(&state, &frame(&["a", "b"]), &ids(&["a", "b", "c"]), &sets(&[&["c"], &["c"]]), true, &mut random);
        assert_eq!(full.current, frame(&["a", "b"]));
        assert_eq!(full.bag, state.bag);
        // Nothing allowed anywhere: the gaps stay.
        let none = fill_slots(&state, &frame(&["a", ""]), &ids(&["a", "b"]), &sets(&[&[], &[]]), true, &mut random);
        assert_eq!(none.current, frame(&["a", ""]));
        assert_eq!(none.history, state.history);
    }

    /// The cases of `apps/web/tests/rotation.test.mjs`, one for one, with the
    /// same ids, the same seeds and the same little generator, so a drift
    /// between the two engines shows up here.
    mod like_the_typescript_tests {
        use super::*;

        fn all() -> Vec<String> {
            ids(&["a", "b", "c", "d", "e"])
        }

        /// The test file's `seeded`: `s = (s * 1664525 + 1013904223) % 2^32`, as a fraction of 2^32.
        fn seeded(seed: u64) -> impl FnMut() -> f64 {
            let mut state = seed;
            move || {
                state = (state * 1_664_525 + 1_013_904_223) % 4_294_967_296;
                state as f64 / 4_294_967_296.0
            }
        }

        #[test]
        fn shuffle_shows_every_wallpaper_once_before_any_repeats() {
            let mut state = EngineState::default();
            let mut random = seeded(7);
            let mut seen = Vec::new();
            for _ in 0..all().len() {
                state = advance(&state, &all(), true, 1, &mut random);
                seen.push(state.current[0].clone());
            }
            seen.sort();
            assert_eq!(seen, all());
        }

        #[test]
        fn a_new_shuffle_round_never_opens_with_the_wallpaper_on_screen() {
            for seed in 1..=40 {
                let mut state = EngineState::default();
                let mut random = seeded(seed);
                let mut previous: Option<String> = None;
                for step in 0..all().len() * 4 {
                    state = advance(&state, &all(), true, 1, &mut random);
                    assert_ne!(Some(&state.current[0]), previous.as_ref(), "seed {seed} repeated at step {step}");
                    previous = Some(state.current[0].clone());
                }
            }
        }

        #[test]
        fn in_order_walks_the_list_and_wraps() {
            let mut state = EngineState::default();
            let mut random = seeded(1);
            let mut shown = Vec::new();
            for _ in 0..7 {
                state = advance(&state, &all(), false, 1, &mut random);
                shown.push(state.current[0].clone());
            }
            assert_eq!(shown, ids(&["a", "b", "c", "d", "e", "a", "b"]));
        }

        #[test]
        fn previous_steps_back_through_what_was_actually_shown() {
            let mut state = EngineState::default();
            let mut random = seeded(3);
            let mut shown = Vec::new();
            for _ in 0..4 {
                state = advance(&state, &all(), true, 1, &mut random);
                shown.push(state.current[0].clone());
            }
            state = back(&state).unwrap();
            assert_eq!(state.current[0], shown[2]);
            state = back(&state).unwrap();
            assert_eq!(state.current[0], shown[1]);
            state = back(&state).unwrap();
            assert_eq!(state.current[0], shown[0]);
            assert!(back(&state).is_none());
        }

        #[test]
        fn picking_a_wallpaper_by_hand_is_remembered_in_history() {
            let mut random = seeded(1);
            let mut state = advance(&EngineState::default(), &all(), false, 1, &mut random);
            state = show(&state, &frame(&["d"]));
            assert_eq!(state.current, frame(&["d"]));
            assert_eq!(back(&state).unwrap().current, frame(&["a"]));
            // In order continues from the hand-picked one.
            assert_eq!(advance(&state, &all(), false, 1, &mut random).current, frame(&["e"]));
        }

        #[test]
        fn removed_wallpapers_are_forgotten_everywhere() {
            let mut random = seeded(1);
            let mut state = EngineState::default();
            for _ in 0..3 {
                state = advance(&state, &all(), false, 1, &mut random);
            }
            state = reconcile(&state, &ids(&["a", "c", "e"]));
            assert_eq!(state.current, frame(&["c"]));
            assert_eq!(state.history, vec![frame(&["a"])]);
        }

        #[test]
        fn each_display_gets_a_different_wallpaper_when_there_are_enough() {
            let mut state = EngineState::default();
            let mut random = seeded(11);
            for _ in 0..12 {
                state = advance(&state, &all(), true, 3, &mut random);
                let distinct: HashSet<_> = state.current.iter().collect();
                assert_eq!(distinct.len(), 3);
            }
            let ordered = advance(&EngineState::default(), &all(), false, 2, &mut random);
            assert_eq!(ordered.current, frame(&["a", "b"]));
            // Fewer wallpapers than displays: repeat rather than stall.
            let scarce = advance(&EngineState::default(), &ids(&["a"]), true, 2, &mut random);
            assert_eq!(scarce.current, frame(&["a", "a"]));
        }

        #[test]
        fn nothing_to_show_leaves_the_state_alone_and_history_is_capped() {
            let empty = EngineState::default();
            let mut random = seeded(1);
            assert_eq!(advance(&empty, &[], true, 1, &mut random), empty);
            let mut state = EngineState::default();
            for _ in 0..HISTORY_LIMIT + 20 {
                state = advance(&state, &all(), false, 1, &mut random);
            }
            assert_eq!(state.history.len(), HISTORY_LIMIT);
        }

        #[test]
        fn a_per_display_frame_keeps_its_positions_when_a_wallpaper_is_removed() {
            let state = EngineState {
                bag: vec![],
                current: frame(&["a", "b"]),
                history: vec![frame(&["c", "d"]), frame(&["b", "b"])],
            };
            let after = reconcile(&state, &ids(&["a", "c", "d"]));
            assert_eq!(after.current, frame(&["a", ""]));
            // A frame with nothing left is dropped; one with something left keeps its shape.
            assert_eq!(after.history, vec![frame(&["c", "d"])]);
            assert!(reconcile(&state, &ids(&["c"])).current.is_empty());
        }

        #[test]
        fn each_display_only_gets_wallpapers_that_are_allowed_on_it() {
            // Display 0 is demanding (only a and b are large enough); display 1 takes anything.
            let allowed = sets(&[&["a", "b"], &["a", "b", "c", "d", "e"]]);
            for shuffle in [true, false] {
                let mut state = EngineState::default();
                let mut random = seeded(5);
                for _ in 0..20 {
                    state = advance_slots(&state, &all(), &allowed, shuffle, &mut random);
                    assert!(allowed[0].contains(&state.current[0]), "display 0 got {}", state.current[0]);
                    assert!(allowed[1].contains(&state.current[1]));
                    assert_ne!(state.current[0], state.current[1]);
                }
            }
        }

        #[test]
        fn a_display_with_nothing_large_enough_is_left_alone() {
            let allowed = sets(&[&[], &["c", "d"]]);
            let mut random = seeded(1);
            let mut state = advance_slots(&EngineState::default(), &all(), &allowed, false, &mut random);
            assert_eq!(state.current, frame(&["", "c"]));
            state = advance_slots(&state, &all(), &allowed, false, &mut random);
            assert_eq!(state.current, frame(&["", "d"]));
            state = advance_slots(&state, &all(), &allowed, false, &mut random);
            assert_eq!(state.current, frame(&["", "c"]));
            // Nothing allowed anywhere: nothing happens.
            let idle = EngineState::default();
            assert_eq!(advance_slots(&idle, &all(), &sets(&[&[], &[]]), true, &mut random), idle);
        }

        #[test]
        fn with_everything_allowed_per_display_frames_match_the_plain_rules() {
            let everything = sets(&[&["a", "b", "c", "d", "e"], &["a", "b", "c", "d", "e"]]);
            let mut random = seeded(9);
            let mut state = EngineState::default();
            let mut shown = Vec::new();
            for _ in 0..3 {
                state = advance_slots(&state, &all(), &everything, false, &mut random);
                shown.push(state.current.clone());
            }
            assert_eq!(shown, vec![frame(&["a", "b"]), frame(&["c", "d"]), frame(&["e", "a"])]);
            // Shuffled: every wallpaper appears before any repeats.
            state = EngineState::default();
            let mut random = seeded(9);
            let mut seen = Vec::new();
            for _ in 0..2 {
                state = advance_slots(&state, &all(), &everything, true, &mut random);
                seen.extend(state.current.clone());
            }
            let distinct: HashSet<_> = seen.iter().collect();
            assert_eq!(distinct.len(), 4);
        }

        #[test]
        fn a_display_with_one_allowed_wallpaper_shares_it_rather_than_going_empty() {
            let allowed = sets(&[&["a"], &["a"]]);
            let mut random = seeded(2);
            let state = advance_slots(&EngineState::default(), &all(), &allowed, true, &mut random);
            assert_eq!(state.current, frame(&["a", "a"]));
        }

        #[test]
        fn in_order_a_demanding_first_display_cannot_freeze_the_rotation() {
            // The real desk: the MacBook comes first and only takes the two largest
            // pictures; the monitor takes those and a smaller one.
            let plan = ids(&["pano", "fourk", "sample", "hd"]);
            let allowed = sets(&[&["pano", "fourk"], &["pano", "fourk", "sample"]]);
            let mut random = seeded(1);
            let mut state = EngineState::default();
            let mut frames = Vec::new();
            for _ in 0..6 {
                state = advance_slots(&state, &plan, &allowed, false, &mut random);
                frames.push(state.current.clone());
            }
            assert_eq!(
                frames,
                vec![
                    frame(&["pano", "fourk"]),
                    frame(&["fourk", "sample"]),
                    frame(&["pano", "fourk"]),
                    frame(&["fourk", "sample"]),
                    frame(&["pano", "fourk"]),
                    frame(&["fourk", "sample"]),
                ]
            );
            // Every display with a choice shows something new each time.
            for pair in frames.windows(2) {
                assert_ne!(pair[1][0], pair[0][0]);
                assert_ne!(pair[1][1], pair[0][1]);
            }
        }
    }

    #[test]
    fn rng_stays_inside_the_unit_interval() {
        let mut source = Rng::from_seed(42);
        for _ in 0..10_000 {
            let value = source.next_f64();
            assert!((0.0..1.0).contains(&value));
        }
    }
}
