//! Which wallpaper comes next. Pure: no clock, no files, no AppKit.
//!
//! A faithful port of `apps/web/src/lib/rotation.ts`, which documents the
//! rules both sides must follow:
//!
//!  - A "frame" is what is on the desktop: one wallpaper id normally, or one
//!    per display when each display shows something different.
//!  - Shuffle draws from a bag holding every wallpaper once, so nothing repeats
//!    until all have been shown. A fresh bag never starts with what is showing.
//!  - In order walks the list and wraps.
//!  - Previous steps back through what was actually shown, shuffled or not.

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

/// After the library changes: forget wallpapers that are gone, keep everything else.
pub fn reconcile(state: &EngineState, ids: &[String]) -> EngineState {
    let keep = |frame: &Vec<String>| -> Vec<String> {
        frame.iter().filter(|id| ids.contains(id)).cloned().collect()
    };
    EngineState {
        bag: keep(&state.bag),
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

/// Reshape the frame that is showing to `count` wallpapers without counting it
/// as a change (no history entry): surplus entries are dropped, missing ones
/// are drawn the way `advance` would draw them. Used when the number of
/// displays or the display mode changes. With no ids to draw from, the frame
/// repeats itself instead (and an empty frame stays empty).
pub fn refit(
    state: &EngineState,
    ids: &[String],
    shuffle: bool,
    count: usize,
    random: &mut dyn FnMut() -> f64,
) -> EngineState {
    let count = count.max(1);
    if state.current.len() == count {
        return state.clone();
    }
    let mut frame = state.current.clone();
    frame.truncate(count);
    let ids = unique(ids);
    if ids.is_empty() {
        if frame.is_empty() {
            return state.clone();
        }
        let seed = frame.clone();
        while frame.len() < count {
            frame.push(seed[frame.len() % seed.len()].clone());
        }
        return EngineState { current: frame, ..state.clone() };
    }
    let last = frame.last().cloned();
    let (bag, frame) = if shuffle {
        // Whatever is already on screen must leave the bag first, or the draw
        // below could keep meeting ids it is not allowed to place.
        let bag = sanitized_bag(&state.bag, &ids, &frame);
        draw_shuffled(bag, frame, last, &ids, count, random)
    } else {
        (state.bag.clone(), draw_in_order(frame, last, &ids, count))
    };
    EngineState { bag, current: frame, history: state.history.clone() }
}

/// Step back to the previously shown frame, or `None` when there is no history.
pub fn back(state: &EngineState) -> Option<EngineState> {
    let mut history = state.history.clone();
    let current = history.pop()?;
    Some(EngineState { bag: state.bag.clone(), current, history })
}

/// The person picked a wallpaper by hand: show it and remember what it replaced.
pub fn show(state: &EngineState, frame: &[String]) -> EngineState {
    if frame.is_empty() {
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
    if state.current.is_empty() {
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
    use std::collections::HashSet;

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
        assert_eq!(kept.current, frame(&["c"]));
        // A frame that loses every id disappears; a partly surviving one stays.
        assert_eq!(kept.history, vec![frame(&["a"]), frame(&["b"])]);
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
    fn refit_tops_up_without_touching_history() {
        let all = ids(&["a", "b", "c", "d"]);
        for seed in 0..100 {
            let mut random = rng(seed);
            // "b" is both on screen and still in the bag, as happens after "previous".
            let state = EngineState {
                bag: ids(&["b"]),
                current: frame(&["b"]),
                history: vec![frame(&["a"])],
            };
            let fitted = refit(&state, &all, true, 3, &mut random);
            assert_eq!(fitted.current.len(), 3);
            assert_eq!(fitted.current[0], "b", "what is showing stays first");
            let distinct: HashSet<_> = fitted.current.iter().collect();
            assert_eq!(distinct.len(), 3, "seed {seed}");
            assert_eq!(fitted.history, state.history);
        }
    }

    #[test]
    fn refit_in_order_continues_after_the_last_item() {
        let all = ids(&["a", "b", "c"]);
        let mut random = rng(0);
        let state = EngineState { current: frame(&["b"]), ..Default::default() };
        let fitted = refit(&state, &all, false, 3, &mut random);
        assert_eq!(fitted.current, frame(&["b", "c", "a"]));
    }

    #[test]
    fn refit_shrinks_and_repeats() {
        let mut random = rng(0);
        let state = EngineState { current: frame(&["a", "b", "c"]), ..Default::default() };
        assert_eq!(refit(&state, &ids(&["a", "b", "c"]), true, 1, &mut random).current, frame(&["a"]));
        // Nothing to draw from: repeat what is there.
        let lone = EngineState { current: frame(&["a", "b"]), ..Default::default() };
        assert_eq!(refit(&lone, &[], false, 5, &mut random).current, frame(&["a", "b", "a", "b", "a"]));
        // Nothing showing and nothing to draw from: nothing to do.
        let empty = EngineState::default();
        assert_eq!(refit(&empty, &[], true, 2, &mut random), empty);
    }

    #[test]
    fn refit_from_nothing_draws_a_whole_frame_without_history() {
        let mut random = rng(4);
        let drawn = refit(&EngineState::default(), &ids(&["a", "b", "c"]), true, 2, &mut random);
        assert_eq!(drawn.current.len(), 2);
        assert_ne!(drawn.current[0], drawn.current[1]);
        assert!(drawn.history.is_empty());
        let in_order = refit(&EngineState::default(), &ids(&["a", "b", "c"]), false, 2, &mut random);
        assert_eq!(in_order.current, frame(&["a", "b"]));
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

    #[test]
    fn rng_stays_inside_the_unit_interval() {
        let mut source = Rng::from_seed(42);
        for _ in 0..10_000 {
            let value = source.next_f64();
            assert!((0.0..1.0).contains(&value));
        }
    }
}
