//! The displays attached to this Mac, in the contract's coordinate system.
//!
//! AppKit measures from the bottom-left corner of the primary display with y
//! growing upward. The UI (and the span maths) want the top-left corner of the
//! primary display with y growing downward, so that is converted here once.

use std::ptr::NonNull;

use block2::RcBlock;
use objc2::rc::Retained;
use objc2::MainThreadMarker;
use objc2_app_kit::{NSApplicationDidChangeScreenParametersNotification, NSScreen};
use objc2_foundation::{ns_string, NSNotification, NSNotificationCenter, NSNumber, NSRect};
use tauri::AppHandle;

use crate::main_thread::on_main;
use crate::model::DisplayInfo;

/// Every screen AppKit knows about. Mirrored displays appear once.
pub fn list(mtm: MainThreadMarker) -> Vec<DisplayInfo> {
    let screens = NSScreen::screens(mtm);
    let frames: Vec<NSRect> = screens.iter().map(|screen| screen.frame()).collect();
    // The primary display is the one AppKit's coordinates are measured from.
    let primary_index = frames
        .iter()
        .position(|frame| frame.origin.x == 0.0 && frame.origin.y == 0.0)
        .unwrap_or(0);
    let primary_height = frames.get(primary_index).map_or(0.0, |frame| frame.size.height);

    screens
        .iter()
        .enumerate()
        .filter_map(|(index, screen)| {
            let id = display_id(&screen)?;
            let frame = frames[index];
            Some(DisplayInfo {
                id: id.to_string(),
                name: screen.localizedName().to_string(),
                x: frame.origin.x,
                y: flip_y(primary_height, frame.origin.y, frame.size.height),
                width: frame.size.width,
                height: frame.size.height,
                scale: screen.backingScaleFactor(),
                primary: index == primary_index,
            })
        })
        .collect()
}

/// The same list, asked for from a background thread.
pub fn fetch(app: &AppHandle) -> Result<Vec<DisplayInfo>, String> {
    on_main(app, list)
}

pub fn find_screen(mtm: MainThreadMarker, id: &str) -> Option<Retained<NSScreen>> {
    NSScreen::screens(mtm)
        .iter()
        .find(|screen| display_id(screen).is_some_and(|found| found.to_string() == id))
}

/// Call `on_change` (on the main thread) whenever displays are connected,
/// removed, rearranged or change resolution. macOS often sends several of
/// these in a burst; the receiver is expected to settle before reacting.
pub fn watch(_mtm: MainThreadMarker, on_change: impl Fn() + 'static) {
    let block = RcBlock::new(move |_notification: NonNull<NSNotification>| on_change());
    // SAFETY: the notification name is an AppKit constant, no object filter is
    // used, and with no queue the block runs on the posting (main) thread,
    // which is where it was created.
    let observer = unsafe {
        NSNotificationCenter::defaultCenter().addObserverForName_object_queue_usingBlock(
            Some(NSApplicationDidChangeScreenParametersNotification),
            None,
            None,
            &block,
        )
    };
    // Observe for as long as the app runs.
    std::mem::forget(observer);
}

/// The order "each display gets its own wallpaper" assigns items in:
/// left to right, then top to bottom.
pub fn reading_order(displays: &[DisplayInfo]) -> Vec<DisplayInfo> {
    let mut ordered = displays.to_vec();
    ordered.sort_by(|a, b| a.x.total_cmp(&b.x).then(a.y.total_cmp(&b.y)).then(a.id.cmp(&b.id)));
    ordered
}

/// AppKit's y (bottom edge, measured upward from the primary display's bottom)
/// to the contract's y (top edge, measured downward from the primary's top).
pub fn flip_y(primary_height: f64, appkit_y: f64, height: f64) -> f64 {
    primary_height - (appkit_y + height)
}

fn display_id(screen: &NSScreen) -> Option<u32> {
    let number = screen.deviceDescription().objectForKey(ns_string!("NSScreenNumber"))?;
    let number = number.downcast::<NSNumber>().ok()?;
    Some(number.unsignedIntValue())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn display(id: &str, x: f64, y: f64) -> DisplayInfo {
        DisplayInfo {
            id: id.to_string(),
            name: id.to_string(),
            x,
            y,
            width: 1920.0,
            height: 1080.0,
            scale: 1.0,
            primary: x == 0.0 && y == 0.0,
        }
    }

    #[test]
    fn the_primary_display_sits_at_the_origin() {
        assert_eq!(flip_y(982.0, 0.0, 982.0), 0.0);
    }

    #[test]
    fn a_display_above_the_primary_has_negative_y() {
        // AppKit: its bottom edge is level with the primary's top edge.
        assert_eq!(flip_y(982.0, 982.0, 1440.0), -1440.0);
    }

    #[test]
    fn a_display_below_the_primary_starts_at_the_primary_height() {
        // AppKit: it hangs entirely below y = 0.
        assert_eq!(flip_y(982.0, -1080.0, 1080.0), 982.0);
    }

    #[test]
    fn a_taller_neighbour_with_aligned_tops_also_has_y_zero() {
        // 1440-tall display beside a 982-tall primary, tops aligned:
        // AppKit puts its bottom 458 points below the primary's bottom.
        assert_eq!(flip_y(982.0, -458.0, 1440.0), 0.0);
    }

    #[test]
    fn reading_order_is_left_to_right_then_top_to_bottom() {
        let displays = vec![
            display("right", 1920.0, 0.0),
            display("below", 0.0, 1080.0),
            display("main", 0.0, 0.0),
            display("left", -1920.0, 200.0),
        ];
        let ids: Vec<String> = reading_order(&displays).into_iter().map(|d| d.id).collect();
        assert_eq!(ids, ["left", "main", "below", "right"]);
    }
}
