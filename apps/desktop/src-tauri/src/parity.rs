//! Parity check, part 2 of 2: replay cases recorded from the TypeScript rules
//! and require the same answers from `engine.rs` and `fit.rs`.
//!
//! Opt-in, because it needs a file produced by `scripts/parity-traces.mjs`
//! (see the top of that script for the two commands). Without
//! `STILL_PARITY_TRACES` in the environment the test passes without checking.

use std::collections::HashSet;

use serde::Deserialize;

use crate::engine::{self, EngineState};
use crate::fit::{self, FitItem, Placement};
use crate::model::{DisplayInfo, DisplayMode, WallpaperKind};

#[derive(Deserialize)]
struct Traces {
    scenarios: Vec<Scenario>,
    fits: Vec<FitCase>,
}

#[derive(Deserialize)]
struct Scenario {
    ids: Vec<String>,
    shuffle: bool,
    seed: u64,
    ops: Vec<Step>,
}

#[derive(Deserialize)]
struct Step {
    #[serde(flatten)]
    op: Op,
    after: EngineState,
}

#[derive(Deserialize)]
#[serde(tag = "kind", rename_all = "camelCase")]
enum Op {
    Advance { count: usize },
    AdvanceSlots { allowed: Vec<Vec<String>> },
    Back,
    Show { frame: Vec<String> },
    Reconcile { alive: Vec<String> },
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct FitCase {
    item: Size,
    displays: Vec<Screen>,
    enlargement_on: Option<f64>,
    enlargement_spanned: Option<f64>,
    fits_display: Option<bool>,
    fits_every: bool,
    fits_any: bool,
    can_span: bool,
    placement_same: String,
    placement_span: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Size {
    kind: WallpaperKind,
    width: f64,
    height: f64,
    #[serde(default)]
    per_display: bool,
}

#[derive(Deserialize)]
struct Screen {
    x: f64,
    y: f64,
    width: f64,
    height: f64,
    scale: f64,
}

/// The generator the TypeScript tests use, so both sides draw the same numbers.
fn seeded(seed: u64) -> impl FnMut() -> f64 {
    let mut state = seed;
    move || {
        state = (state * 1_664_525 + 1_013_904_223) % 4_294_967_296;
        state as f64 / 4_294_967_296.0
    }
}

fn word(placement: Placement) -> &'static str {
    match placement {
        Placement::Span => "span",
        Placement::Same => "same",
        Placement::Skip => "skip",
    }
}

#[test]
fn rust_gives_the_same_answers_as_the_typescript_rules() {
    let Ok(path) = std::env::var("STILL_PARITY_TRACES") else {
        eprintln!("parity check skipped: set STILL_PARITY_TRACES (see scripts/parity-traces.mjs)");
        return;
    };
    let text = std::fs::read_to_string(&path).expect("the traces file should be readable");
    let traces: Traces = serde_json::from_str(&text).expect("the traces file should be the script's output");

    let mut steps = 0;
    for (number, scenario) in traces.scenarios.iter().enumerate() {
        let mut random = seeded(scenario.seed);
        let mut state = EngineState::default();
        for (index, step) in scenario.ops.iter().enumerate() {
            state = match &step.op {
                Op::Advance { count } => engine::advance(&state, &scenario.ids, scenario.shuffle, *count, &mut random),
                Op::AdvanceSlots { allowed } => {
                    let allowed: Vec<HashSet<String>> =
                        allowed.iter().map(|list| list.iter().cloned().collect()).collect();
                    engine::advance_slots(&state, &scenario.ids, &allowed, scenario.shuffle, &mut random)
                }
                Op::Back => engine::back(&state).unwrap_or(state),
                Op::Show { frame } => engine::show(&state, frame),
                Op::Reconcile { alive } => engine::reconcile(&state, alive),
            };
            assert_eq!(state, step.after, "scenario {number}, step {index}");
            steps += 1;
        }
    }

    for (number, case) in traces.fits.iter().enumerate() {
        let item = FitItem {
            kind: case.item.kind,
            width: case.item.width,
            height: case.item.height,
            per_display: case.item.per_display,
        };
        let displays: Vec<DisplayInfo> = case
            .displays
            .iter()
            .enumerate()
            .map(|(index, screen)| DisplayInfo {
                id: index.to_string(),
                name: String::new(),
                x: screen.x,
                y: screen.y,
                width: screen.width,
                height: screen.height,
                scale: screen.scale,
                primary: index == 0,
            })
            .collect();
        let close = |ours: f64, theirs: f64| (ours - theirs).abs() <= theirs.abs() * 1e-12;
        if let (Some(expected), Some(first)) = (case.enlargement_on, displays.first()) {
            assert!(close(fit::enlargement_on(&item, first), expected), "size case {number}: enlargement on a display");
        }
        if let Some(expected) = case.enlargement_spanned {
            assert!(close(fit::enlargement_spanned(&item, &displays), expected), "size case {number}: enlargement spanned");
        }
        if let (Some(expected), Some(first)) = (case.fits_display, displays.first()) {
            assert_eq!(fit::fits_display(&item, first), expected, "size case {number}: fits a display");
        }
        assert_eq!(fit::fits_every(&item, &displays), case.fits_every, "size case {number}: fits every");
        assert_eq!(fit::fits_any(&item, &displays), case.fits_any, "size case {number}: fits any");
        assert_eq!(fit::can_span(&item, &displays), case.can_span, "size case {number}: can span");
        assert_eq!(word(fit::placement_for(&item, &displays, DisplayMode::Same)), case.placement_same, "size case {number}");
        assert_eq!(word(fit::placement_for(&item, &displays, DisplayMode::Span)), case.placement_span, "size case {number}");
    }
    eprintln!("parity check: {steps} engine steps and {} size cases match the TypeScript", traces.fits.len());
}
