//! Is a wallpaper large enough for the displays it would be shown on? Pure.
//!
//! A port of `apps/web/src/lib/fit.ts`; the constants, the functions and the
//! tests mirror that file so both sides give the same answers.
//!
//! A wallpaper fills its display ("cover"): it is scaled until both sides are
//! covered and the overflow is cropped. If the picture has fewer pixels than
//! the display, that scaling is an enlargement and the result looks soft.
//! These rules decide when that is acceptable, so the app never puts a visibly
//! blurry picture on the desktop by itself, and never stretches a small
//! picture across several displays.

use crate::model::{DisplayInfo, DisplayMode, WallpaperKind};

/// A still picture may be enlarged by up to a quarter before it counts as too small.
pub const SHARP_LIMIT: f64 = 1.25;
/// Moving pictures hide softness better: 1080p video is accepted on a 16-inch
/// Retina display, 720p is not.
pub const VIDEO_LIMIT: f64 = 2.2;

/// What the rules need to know about a wallpaper.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct FitItem {
    pub kind: WallpaperKind,
    /// Pixels of the original. 0 (or less) means unknown.
    pub width: f64,
    pub height: f64,
}

/// What automatic rotation does with a wallpaper when every display shows the
/// same thing ("same") or one picture is stretched across all ("span").
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Placement {
    /// Stretch it across the displays.
    Span,
    /// Show it on each display (in span mode this is the fallback for a
    /// picture that is fine per display but too small to stretch).
    Same,
    /// Too small for at least one display, so it is not used.
    Skip,
}

pub fn limit_for(kind: WallpaperKind) -> f64 {
    if kind == WallpaperKind::Video {
        VIDEO_LIMIT
    } else {
        SHARP_LIMIT
    }
}

/// Scenes are drawn at any size, and a picture of unknown size cannot be
/// judged: both always pass. Written as "not greater than zero" so a
/// not-a-number size counts as unknown, exactly as in the TypeScript.
#[allow(clippy::neg_cmp_op_on_partial_ord)]
fn always_fits(item: &FitItem) -> bool {
    item.kind == WallpaperKind::Scene || !(item.width > 0.0) || !(item.height > 0.0)
}

/// How much the picture is enlarged to fill this display.
/// 1 means pixel for pixel, 2 means every picture pixel covers four display pixels.
pub fn enlargement_on(item: &FitItem, display: &DisplayInfo) -> f64 {
    let across = (display.width * display.scale) / item.width;
    let down = (display.height * display.scale) / item.height;
    across.max(down)
}

/// The worst enlargement on any display when one picture covers all of them as
/// a single canvas (their bounding box). A dense display next to an ordinary
/// one needs far more pixels than its share of the width suggests.
pub fn enlargement_spanned(item: &FitItem, displays: &[DisplayInfo]) -> f64 {
    if displays.is_empty() {
        return 0.0;
    }
    let least = |values: &mut dyn Iterator<Item = f64>| values.fold(f64::INFINITY, f64::min);
    let most = |values: &mut dyn Iterator<Item = f64>| values.fold(f64::NEG_INFINITY, f64::max);
    let left = least(&mut displays.iter().map(|display| display.x));
    let top = least(&mut displays.iter().map(|display| display.y));
    let right = most(&mut displays.iter().map(|display| display.x + display.width));
    let bottom = most(&mut displays.iter().map(|display| display.y + display.height));
    // Points of canvas covered by one picture pixel.
    let points_per_pixel = ((right - left) / item.width).max((bottom - top) / item.height);
    points_per_pixel * most(&mut displays.iter().map(|display| display.scale))
}

pub fn fits_display(item: &FitItem, display: &DisplayInfo) -> bool {
    always_fits(item) || enlargement_on(item, display) <= limit_for(item.kind)
}

pub fn fits_every(item: &FitItem, displays: &[DisplayInfo]) -> bool {
    displays.iter().all(|display| fits_display(item, display))
}

/// Only the UI's summary line asks this question; it is ported (and tested)
/// so the two files stay function for function.
#[allow(dead_code)]
pub fn fits_any(item: &FitItem, displays: &[DisplayInfo]) -> bool {
    displays.is_empty() || displays.iter().any(|display| fits_display(item, display))
}

/// Large enough to be stretched across all displays as one picture. Needs at least two displays.
pub fn can_span(item: &FitItem, displays: &[DisplayInfo]) -> bool {
    if displays.len() < 2 {
        return false;
    }
    always_fits(item) || enlargement_spanned(item, displays) <= limit_for(item.kind)
}

/// See [`Placement`]. Only `DisplayMode::Span` asks for stretching; any other
/// mode is judged as "same" (the TypeScript takes just those two modes).
pub fn placement_for(item: &FitItem, displays: &[DisplayInfo], mode: DisplayMode) -> Placement {
    if mode == DisplayMode::Span && can_span(item, displays) {
        return Placement::Span;
    }
    if fits_every(item, displays) {
        Placement::Same
    } else {
        Placement::Skip
    }
}

#[cfg(test)]
mod tests {
    //! One test per case in `apps/web/tests/fit.test.mjs`, same names in
    //! spirit and the same numbers.

    use super::*;

    fn display(x: f64, y: f64, width: f64, height: f64, scale: f64) -> DisplayInfo {
        DisplayInfo { id: format!("{x},{y}"), name: String::new(), x, y, width, height, scale, primary: x == 0.0 && y == 0.0 }
    }

    // A real desk: a 16-inch MacBook Pro beside a 24-inch 1080p monitor.
    fn macbook() -> DisplayInfo {
        display(-1728.0, 0.0, 1728.0, 1117.0, 2.0) // 3456 x 2234 pixels
    }

    fn monitor() -> DisplayInfo {
        display(0.0, 0.0, 1920.0, 1080.0, 1.0)
    }

    fn desk() -> Vec<DisplayInfo> {
        vec![macbook(), monitor()]
    }

    fn image(width: f64, height: f64) -> FitItem {
        FitItem { kind: WallpaperKind::Image, width, height }
    }

    fn video(width: f64, height: f64) -> FitItem {
        FitItem { kind: WallpaperKind::Video, width, height }
    }

    fn four_k() -> FitItem {
        image(3840.0, 2400.0)
    }

    fn panorama() -> FitItem {
        image(7680.0, 2400.0)
    }

    fn sample() -> FitItem {
        image(1672.0, 941.0)
    }

    fn hd720() -> FitItem {
        image(1280.0, 720.0)
    }

    fn near(actual: f64, expected: f64) {
        assert!((actual - expected).abs() < 0.005, "{actual} vs {expected}");
    }

    /// The UI's one-line rotation summary (`rotationUse` in fit.ts), rebuilt
    /// from the ported functions: `(spanned, shown, skipped)`.
    fn rotation_use(items: &[FitItem], displays: &[DisplayInfo], mode: DisplayMode) -> (usize, usize, usize) {
        let (mut spanned, mut shown, mut skipped) = (0, 0, 0);
        for item in items {
            if mode == DisplayMode::Each {
                // Each display draws from the wallpapers that are large enough for it.
                if fits_any(item, displays) {
                    shown += 1;
                } else {
                    skipped += 1;
                }
                continue;
            }
            match placement_for(item, displays, mode) {
                Placement::Span => spanned += 1,
                Placement::Same => shown += 1,
                Placement::Skip => skipped += 1,
            }
        }
        (spanned, shown, skipped)
    }

    #[test]
    fn enlargement_is_measured_against_the_displays_real_pixels() {
        near(enlargement_on(&four_k(), &macbook()), 0.931); // 2234 / 2400
        near(enlargement_on(&sample(), &macbook()), 2.374); // 2234 / 941
        near(enlargement_on(&sample(), &monitor()), 1.148); // 1920 / 1672
        near(enlargement_on(&hd720(), &monitor()), 1.5);
    }

    #[test]
    fn a_picture_fits_a_display_when_it_is_enlarged_by_a_quarter_at_most() {
        assert_eq!(SHARP_LIMIT, 1.25);
        assert!(fits_display(&four_k(), &macbook()));
        assert!(fits_display(&four_k(), &monitor()));
        assert!(!fits_display(&sample(), &macbook()));
        assert!(fits_display(&sample(), &monitor()));
        assert!(!fits_display(&hd720(), &monitor()));
        assert!(fits_display(&image(2880.0, 1800.0), &macbook())); // 1.24
        assert!(!fits_display(&image(2560.0, 1440.0), &macbook())); // 1.55
    }

    #[test]
    fn fitting_every_display_and_fitting_any_display_are_different_questions() {
        assert!(fits_every(&four_k(), &desk()));
        assert!(!fits_every(&sample(), &desk()));
        assert!(fits_any(&sample(), &desk()));
        assert!(!fits_any(&hd720(), &desk()));
        assert!(fits_any(&hd720(), &[]));
    }

    #[test]
    fn stretching_across_displays_is_judged_on_the_densest_display() {
        // Canvas 3648 x 1117 points. The panorama gives 0.475 points per pixel: 0.95 on the Retina side.
        near(enlargement_spanned(&panorama(), &desk()), 0.95);
        // A 4K picture gives 0.95 points per pixel: 1.9 on the Retina side.
        near(enlargement_spanned(&four_k(), &desk()), 1.9);
        assert!(can_span(&panorama(), &desk()));
        assert!(!can_span(&four_k(), &desk()));
        assert!(!can_span(&sample(), &desk()));
    }

    #[test]
    fn two_ordinary_monitors_need_far_fewer_pixels_to_span() {
        let pair = [monitor(), display(1920.0, 0.0, 1920.0, 1080.0, 1.0)];
        near(enlargement_spanned(&four_k(), &pair), 1.0); // 3840 points over 3840 pixels
        assert!(can_span(&four_k(), &pair));
        assert!(!can_span(&image(1920.0, 1080.0), &pair));
    }

    #[test]
    fn displays_stacked_or_offset_use_the_whole_bounding_box() {
        let stacked = [monitor(), display(200.0, 1080.0, 1920.0, 1080.0, 1.0)];
        // Box 2120 x 2160 points; a 3840 x 2400 picture covers it at 0.9 points per pixel.
        near(enlargement_spanned(&four_k(), &stacked), 0.9);
    }

    #[test]
    fn one_display_can_never_be_spanned() {
        assert!(!can_span(&panorama(), &[macbook()]));
        assert!(!can_span(&FitItem { kind: WallpaperKind::Scene, width: 0.0, height: 0.0 }, &[macbook()]));
    }

    #[test]
    fn rotation_spans_what_it_can_shows_the_rest_on_each_display_and_skips_what_is_too_small() {
        assert_eq!(placement_for(&panorama(), &desk(), DisplayMode::Span), Placement::Span);
        assert_eq!(placement_for(&four_k(), &desk(), DisplayMode::Span), Placement::Same);
        assert_eq!(placement_for(&sample(), &desk(), DisplayMode::Span), Placement::Skip);
        assert_eq!(placement_for(&panorama(), &desk(), DisplayMode::Same), Placement::Same);
        assert_eq!(placement_for(&sample(), &desk(), DisplayMode::Same), Placement::Skip);
        assert_eq!(placement_for(&sample(), &[monitor()], DisplayMode::Same), Placement::Same);
    }

    #[test]
    fn video_is_allowed_more_enlargement_than_a_still_picture() {
        assert!(fits_display(&video(1920.0, 1080.0), &macbook())); // 2.07
        assert!(!fits_display(&video(1280.0, 720.0), &macbook())); // 3.1
        assert!(fits_display(&video(1280.0, 720.0), &monitor())); // 1.5
    }

    #[test]
    fn scenes_and_pictures_of_unknown_size_always_pass() {
        let scene = FitItem { kind: WallpaperKind::Scene, width: 1920.0, height: 1200.0 };
        assert!(fits_every(&scene, &desk()));
        assert!(can_span(&scene, &desk()));
        assert_eq!(placement_for(&scene, &desk(), DisplayMode::Span), Placement::Span);
        let unknown = image(0.0, 0.0);
        assert!(fits_every(&unknown, &desk()));
        assert!(can_span(&unknown, &desk()));
    }

    #[test]
    fn the_rotation_summary_counts_each_outcome() {
        let library = [panorama(), panorama(), four_k(), four_k(), four_k(), sample(), hd720()];
        assert_eq!(rotation_use(&library, &desk(), DisplayMode::Span), (2, 3, 2));
        assert_eq!(rotation_use(&library, &desk(), DisplayMode::Same), (0, 5, 2));
        // With a different wallpaper per display the sample is usable on the monitor.
        assert_eq!(rotation_use(&library, &desk(), DisplayMode::Each), (0, 6, 1));
    }

    // Not in the TypeScript file: the edges the port has to get right by hand.

    #[test]
    fn a_size_that_is_not_a_number_counts_as_unknown() {
        let odd = image(f64::NAN, 941.0);
        assert!(fits_every(&odd, &desk()));
        assert!(fits_display(&image(-5.0, 100.0), &macbook()));
        assert!(fits_display(&video(0.0, 0.0), &macbook()), "a video of unknown size passes");
    }

    #[test]
    fn no_displays_means_no_enlargement() {
        assert_eq!(enlargement_spanned(&four_k(), &[]), 0.0);
        assert!(fits_every(&sample(), &[]));
    }
}
