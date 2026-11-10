//! Putting still pictures on the desktop.
//!
//! Three parts:
//!  - span geometry (pure): which part of one picture belongs on each display
//!    when all displays are treated as a single canvas;
//!  - span rendering: cutting those parts out into files macOS can show;
//!  - `set_pictures`: the one place that asks macOS to change a desktop picture.

use std::fs;
use std::io::BufWriter;
use std::path::{Path, PathBuf};
use std::time::UNIX_EPOCH;

use image::codecs::jpeg::JpegEncoder;
use image::imageops::{self, FilterType};
use image::{DynamicImage, ImageDecoder, ImageEncoder, ImageFormat, ImageReader, RgbImage};
use objc2::runtime::AnyObject;
use objc2_app_kit::{
    NSImageScaling, NSWorkspace, NSWorkspaceDesktopImageAllowClippingKey, NSWorkspaceDesktopImageScalingKey,
};
use objc2_foundation::{NSDictionary, NSNumber, NSString, NSURL};
use tauri::AppHandle;

use crate::displays;
use crate::main_thread::on_main;
use crate::model::DisplayInfo;

const PIECE_PREFIX: &str = "span-";
const JPEG_QUALITY: u8 = 92;

// ---------------------------------------------------------------------------
// Span geometry
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Rect {
    pub x: f64,
    pub y: f64,
    pub width: f64,
    pub height: f64,
}

/// One display's share of a spanned picture.
#[derive(Debug, Clone, PartialEq)]
pub struct SpanPiece {
    pub display_id: String,
    /// The part of the source picture under this display, in source pixels.
    /// Fractional; `pixel_crop` snaps it to whole pixels.
    pub crop: Rect,
    /// The size to render at: the display's size in real pixels.
    pub out_width: u32,
    pub out_height: u32,
}

/// The smallest rectangle (in points) that contains every display.
pub fn canvas_bounds(displays: &[DisplayInfo]) -> Option<Rect> {
    let first = displays.first()?;
    let mut left = first.x;
    let mut top = first.y;
    let mut right = first.x + first.width;
    let mut bottom = first.y + first.height;
    for display in &displays[1..] {
        left = left.min(display.x);
        top = top.min(display.y);
        right = right.max(display.x + display.width);
        bottom = bottom.max(display.y + display.height);
    }
    Some(Rect { x: left, y: top, width: right - left, height: bottom - top })
}

/// Where a display sits inside the canvas, in points from the canvas's top-left.
pub fn offset_in_canvas(canvas: &Rect, display: &DisplayInfo) -> (f64, f64) {
    (display.x - canvas.x, display.y - canvas.y)
}

/// Lay one picture across all displays as a single canvas.
///
/// The picture is scaled to cover the canvas (keeping its proportions, centred,
/// overflow cut off) and each display gets the part that lies under it, to be
/// rendered at that display's pixel size. Displays with different pixel
/// densities therefore show the picture at the same physical scale.
pub fn span_layout(image_width: u32, image_height: u32, displays: &[DisplayInfo]) -> Vec<SpanPiece> {
    let Some(canvas) = canvas_bounds(displays) else {
        return Vec::new();
    };
    if image_width == 0 || image_height == 0 || canvas.width <= 0.0 || canvas.height <= 0.0 {
        return Vec::new();
    }
    let image_width = f64::from(image_width);
    let image_height = f64::from(image_height);
    // Points of canvas per source pixel. "Cover" takes the larger ratio so no
    // edge of the canvas is left empty.
    let cover = (canvas.width / image_width).max(canvas.height / image_height);
    // Top-left of the scaled picture relative to the canvas (zero or negative).
    let left = (canvas.width - image_width * cover) / 2.0;
    let top = (canvas.height - image_height * cover) / 2.0;

    displays
        .iter()
        .map(|display| {
            let (offset_x, offset_y) = offset_in_canvas(&canvas, display);
            SpanPiece {
                display_id: display.id.clone(),
                crop: Rect {
                    x: (offset_x - left) / cover,
                    y: (offset_y - top) / cover,
                    width: display.width / cover,
                    height: display.height / cover,
                },
                out_width: pixels(display.width * display.scale),
                out_height: pixels(display.height * display.scale),
            }
        })
        .collect()
}

/// Snap a fractional crop to whole source pixels: `(x, y, width, height)`.
/// Edges are rounded independently, so two displays that share an edge share
/// the same pixel column and the picture continues without a seam or overlap.
pub fn pixel_crop(crop: &Rect, image_width: u32, image_height: u32) -> (u32, u32, u32, u32) {
    let snap = |start: f64, length: f64, limit: u32| -> (u32, u32) {
        let limit = limit.max(1);
        let from = (start.round().max(0.0) as u32).min(limit - 1);
        let to = ((start + length).round().max(0.0) as u32).clamp(from + 1, limit);
        (from, to - from)
    };
    let (x, width) = snap(crop.x, crop.width, image_width);
    let (y, height) = snap(crop.y, crop.height, image_height);
    (x, y, width, height)
}

fn pixels(points: f64) -> u32 {
    points.round().max(1.0) as u32
}

// ---------------------------------------------------------------------------
// Span rendering
// ---------------------------------------------------------------------------

/// Cut `source` into one JPEG per display inside `cache_dir` and return
/// `(display id, file)` pairs.
///
/// macOS does not refresh a desktop picture whose path is unchanged, so each
/// file name carries a hash of everything that shapes the piece. The same hash
/// lets a repeat (displays re-detected, app restarted) reuse the files without
/// decoding the picture again.
pub fn render_span(
    source: &Path,
    item_id: &str,
    displays: &[DisplayInfo],
    cache_dir: &Path,
) -> Result<Vec<(String, PathBuf)>, String> {
    let canvas = canvas_bounds(displays).ok_or_else(|| "No display is connected.".to_string())?;
    fs::create_dir_all(cache_dir)
        .map_err(|error| format!("The wallpaper cache folder could not be created: {error}."))?;
    let stamp = source_stamp(source)?;

    let targets: Vec<(String, PathBuf)> = displays
        .iter()
        .map(|display| {
            let key = format!(
                "{item_id}|{}|{stamp}|{}|{:?}|{:?}",
                source.display(),
                display.id,
                (display.x, display.y, display.width, display.height, display.scale),
                (canvas.x, canvas.y, canvas.width, canvas.height),
            );
            let name = format!("{PIECE_PREFIX}{:016x}.jpg", fnv1a(key.as_bytes()));
            (display.id.clone(), cache_dir.join(name))
        })
        .collect();

    if targets.iter().all(|(_, path)| path.is_file()) {
        return Ok(targets);
    }

    let (picture, color_profile) = decode(source)?;
    let pieces = span_layout(picture.width(), picture.height(), displays);
    for (piece, (_, path)) in pieces.iter().zip(&targets) {
        let (x, y, width, height) = pixel_crop(&piece.crop, picture.width(), picture.height());
        let cut = imageops::crop_imm(&picture, x, y, width, height).to_image();
        let sized = if (cut.width(), cut.height()) == (piece.out_width, piece.out_height) {
            cut
        } else {
            imageops::resize(&cut, piece.out_width, piece.out_height, FilterType::Lanczos3)
        };
        write_jpeg(&sized, color_profile.as_deref(), path)?;
    }
    Ok(targets)
}

/// Delete span pieces that are no longer on any desktop. Only call this after
/// the pieces in `keep` have actually been set: macOS reads a desktop picture
/// from its file again later (after a restart, for instance), so a file that
/// is still showing must stay.
pub fn prune_span_cache(cache_dir: &Path, keep: &[PathBuf]) {
    let Ok(entries) = fs::read_dir(cache_dir) else {
        return;
    };
    for entry in entries.flatten() {
        let path = entry.path();
        let is_piece = entry.file_name().to_string_lossy().starts_with(PIECE_PREFIX);
        if is_piece && !keep.contains(&path) {
            let _ = fs::remove_file(path);
        }
    }
}

/// Decode a JPEG, PNG or WebP into 8-bit RGB, upright, with its colour profile.
fn decode(source: &Path) -> Result<(RgbImage, Option<Vec<u8>>), String> {
    let unreadable = |error: &dyn std::fmt::Display| format!("That picture could not be read: {error}.");
    let reader = ImageReader::open(source)
        .and_then(|reader| reader.with_guessed_format())
        .map_err(|error| unreadable(&error))?;
    if !matches!(reader.format(), Some(ImageFormat::Jpeg | ImageFormat::Png | ImageFormat::WebP)) {
        return Err("Only JPEG, PNG and WebP pictures can be spread across displays.".to_string());
    }
    let mut decoder = reader.into_decoder().map_err(|error| unreadable(&error))?;
    // Photos straight from a camera or phone are often stored sideways with a
    // note saying how to turn them; macOS honours that note, so we must too.
    let orientation = decoder.orientation().ok();
    // Without the profile a wide-gamut (Display P3) photo would come out dull.
    let color_profile = decoder.icc_profile().ok().flatten();
    let mut picture = DynamicImage::from_decoder(decoder).map_err(|error| unreadable(&error))?;
    if let Some(orientation) = orientation {
        picture.apply_orientation(orientation);
    }
    Ok((picture.into_rgb8(), color_profile))
}

fn write_jpeg(picture: &RgbImage, color_profile: Option<&[u8]>, destination: &Path) -> Result<(), String> {
    let failed = |error: &dyn std::fmt::Display| format!("The wallpaper piece could not be written: {error}.");
    // Written beside the destination and renamed, so macOS never sees half a file.
    let temp = destination.with_extension(format!("{}.part", std::process::id()));
    let result = (|| -> Result<(), String> {
        let file = fs::File::create(&temp).map_err(|error| failed(&error))?;
        let mut encoder = JpegEncoder::new_with_quality(BufWriter::new(file), JPEG_QUALITY);
        if let Some(profile) = color_profile {
            // Not fatal: a piece without its profile is still a usable picture.
            let _ = encoder.set_icc_profile(profile.to_vec());
        }
        encoder
            .write_image(picture.as_raw(), picture.width(), picture.height(), image::ExtendedColorType::Rgb8)
            .map_err(|error| failed(&error))?;
        fs::rename(&temp, destination).map_err(|error| failed(&error))
    })();
    if result.is_err() {
        let _ = fs::remove_file(&temp);
    }
    result
}

/// Size and modification time: enough to notice a file replaced under the same name.
fn source_stamp(source: &Path) -> Result<String, String> {
    let metadata = fs::metadata(source).map_err(|_| "That file is not in the library.".to_string())?;
    let modified = metadata
        .modified()
        .ok()
        .and_then(|time| time.duration_since(UNIX_EPOCH).ok())
        .map_or(0, |since| since.as_nanos());
    Ok(format!("{}:{modified}", metadata.len()))
}

/// FNV-1a, written out because the hash must stay the same between app
/// versions (std's hasher makes no such promise) and needs no dependency.
fn fnv1a(bytes: &[u8]) -> u64 {
    let mut hash: u64 = 0xcbf2_9ce4_8422_2325;
    for byte in bytes {
        hash ^= u64::from(*byte);
        hash = hash.wrapping_mul(0x0000_0100_0000_01b3);
    }
    hash
}

// ---------------------------------------------------------------------------
// Asking macOS
// ---------------------------------------------------------------------------

/// Set each `(display id, picture file)` as that display's desktop picture,
/// scaled to fill the screen. Displays that have just been unplugged are skipped.
pub fn set_pictures(app: &AppHandle, pictures: Vec<(String, PathBuf)>) -> Result<(), String> {
    if pictures.is_empty() {
        return Ok(());
    }
    if crate::dry_run() {
        for (display_id, path) in &pictures {
            eprintln!("still (dry run): would set {} on display {display_id}", path.display());
        }
        return Ok(());
    }
    on_main(app, move |mtm| {
        let workspace = NSWorkspace::sharedWorkspace();
        // "Fill": scale proportionally, up or down, and let the overflow be clipped.
        let scaling = NSNumber::new_usize(NSImageScaling::ScaleProportionallyUpOrDown.0);
        let allow_clipping = NSNumber::new_bool(true);
        let scaling: &AnyObject = &scaling;
        let allow_clipping: &AnyObject = &allow_clipping;
        // SAFETY: these are AppKit's documented option keys with the NSNumber values they expect.
        let options: objc2::rc::Retained<NSDictionary<NSString, AnyObject>> = unsafe {
            NSDictionary::from_slices(
                &[NSWorkspaceDesktopImageScalingKey, NSWorkspaceDesktopImageAllowClippingKey],
                &[scaling, allow_clipping],
            )
        };

        let mut first_error: Option<String> = None;
        for (display_id, path) in &pictures {
            let Some(screen) = displays::find_screen(mtm, display_id) else {
                continue;
            };
            let url = NSURL::fileURLWithPath(&NSString::from_str(&path.to_string_lossy()));
            // SAFETY: `options` holds the key and value types this method documents.
            let outcome = unsafe { workspace.setDesktopImageURL_forScreen_options_error(&url, &screen, &options) };
            if let Err(error) = outcome {
                first_error
                    .get_or_insert_with(|| format!("macOS would not set the wallpaper: {}", error.localizedDescription()));
            }
        }
        first_error.map_or(Ok(()), Err)
    })?
}

#[cfg(test)]
mod tests {
    use super::*;

    fn display(id: &str, x: f64, y: f64, width: f64, height: f64, scale: f64) -> DisplayInfo {
        DisplayInfo {
            id: id.to_string(),
            name: id.to_string(),
            x,
            y,
            width,
            height,
            scale,
            primary: x == 0.0 && y == 0.0,
        }
    }

    fn assert_rect(actual: &Rect, expected: (f64, f64, f64, f64)) {
        let close = |a: f64, b: f64| (a - b).abs() < 1e-6;
        assert!(
            close(actual.x, expected.0)
                && close(actual.y, expected.1)
                && close(actual.width, expected.2)
                && close(actual.height, expected.3),
            "expected {expected:?}, got {actual:?}"
        );
    }

    /// Checks that hold for every layout: each crop lies inside the picture,
    /// keeps its display's proportions, and all crops use one common scale.
    fn assert_sane(image: (u32, u32), displays: &[DisplayInfo], pieces: &[SpanPiece]) {
        assert_eq!(pieces.len(), displays.len());
        let scale = pieces[0].crop.width / displays[0].width;
        for (piece, display) in pieces.iter().zip(displays) {
            assert_eq!(piece.display_id, display.id);
            assert!(piece.crop.x >= -1e-6 && piece.crop.y >= -1e-6, "{piece:?}");
            assert!(piece.crop.x + piece.crop.width <= f64::from(image.0) + 1e-6, "{piece:?}");
            assert!(piece.crop.y + piece.crop.height <= f64::from(image.1) + 1e-6, "{piece:?}");
            assert!((piece.crop.width / display.width - scale).abs() < 1e-9);
            assert!((piece.crop.height / display.height - scale).abs() < 1e-9);
            assert_eq!(piece.out_width, (display.width * display.scale).round() as u32);
            assert_eq!(piece.out_height, (display.height * display.scale).round() as u32);
        }
    }

    #[test]
    fn two_equal_displays_side_by_side_split_the_picture_in_half() {
        let displays = [display("left", 0.0, 0.0, 1920.0, 1080.0, 1.0), display("right", 1920.0, 0.0, 1920.0, 1080.0, 1.0)];
        let pieces = span_layout(3840, 1080, &displays);
        assert_sane((3840, 1080), &displays, &pieces);
        assert_rect(&pieces[0].crop, (0.0, 0.0, 1920.0, 1080.0));
        assert_rect(&pieces[1].crop, (1920.0, 0.0, 1920.0, 1080.0));
        assert_eq!((pieces[0].out_width, pieces[0].out_height), (1920, 1080));
        assert_eq!(pixel_crop(&pieces[0].crop, 3840, 1080), (0, 0, 1920, 1080));
        assert_eq!(pixel_crop(&pieces[1].crop, 3840, 1080), (1920, 0, 1920, 1080));
    }

    #[test]
    fn mixed_scale_factors_keep_one_physical_scale_but_render_at_native_pixels() {
        // A Retina laptop panel beside a non-Retina monitor, tops aligned.
        let displays = [display("laptop", 0.0, 0.0, 1512.0, 982.0, 2.0), display("monitor", 1512.0, 0.0, 2560.0, 1440.0, 1.0)];
        assert_rect(&canvas_bounds(&displays).unwrap(), (0.0, 0.0, 4072.0, 1440.0));
        // A picture exactly twice the canvas: half a point per source pixel.
        let pieces = span_layout(8144, 2880, &displays);
        assert_sane((8144, 2880), &displays, &pieces);
        assert_rect(&pieces[0].crop, (0.0, 0.0, 3024.0, 1964.0));
        assert_rect(&pieces[1].crop, (3024.0, 0.0, 5120.0, 2880.0));
        // Same crop density, different output density.
        assert_eq!((pieces[0].out_width, pieces[0].out_height), (3024, 1964));
        assert_eq!((pieces[1].out_width, pieces[1].out_height), (2560, 1440));
    }

    #[test]
    fn a_display_above_another_takes_the_top_of_the_picture() {
        let displays = [display("lower", 0.0, 0.0, 1920.0, 1080.0, 1.0), display("upper", 0.0, -1080.0, 1920.0, 1080.0, 1.0)];
        assert_rect(&canvas_bounds(&displays).unwrap(), (0.0, -1080.0, 1920.0, 2160.0));
        let pieces = span_layout(1920, 2160, &displays);
        assert_sane((1920, 2160), &displays, &pieces);
        assert_rect(&pieces[0].crop, (0.0, 1080.0, 1920.0, 1080.0));
        assert_rect(&pieces[1].crop, (0.0, 0.0, 1920.0, 1080.0));
    }

    #[test]
    fn a_display_with_negative_x_takes_the_left_of_the_picture() {
        let displays = [display("main", 0.0, 0.0, 1920.0, 1080.0, 1.0), display("left", -1920.0, 0.0, 1920.0, 1080.0, 1.0)];
        let canvas = canvas_bounds(&displays).unwrap();
        assert_rect(&canvas, (-1920.0, 0.0, 3840.0, 1080.0));
        assert_eq!(offset_in_canvas(&canvas, &displays[0]), (1920.0, 0.0));
        assert_eq!(offset_in_canvas(&canvas, &displays[1]), (0.0, 0.0));
        let pieces = span_layout(3840, 1080, &displays);
        assert_sane((3840, 1080), &displays, &pieces);
        assert_rect(&pieces[0].crop, (1920.0, 0.0, 1920.0, 1080.0));
        assert_rect(&pieces[1].crop, (0.0, 0.0, 1920.0, 1080.0));
    }

    #[test]
    fn a_picture_wider_than_the_canvas_loses_its_sides_evenly() {
        let displays = [display("left", 0.0, 0.0, 1920.0, 1080.0, 1.0), display("right", 1920.0, 0.0, 1920.0, 1080.0, 1.0)];
        // Canvas is 3840x1080 (32:9); the picture is 8:1, so height decides the scale.
        let pieces = span_layout(8000, 1000, &displays);
        assert_sane((8000, 1000), &displays, &pieces);
        let cover = 1080.0 / 1000.0;
        let hidden_each_side = (8000.0 - 3840.0 / cover) / 2.0;
        assert_rect(&pieces[0].crop, (hidden_each_side, 0.0, 1920.0 / cover, 1000.0));
        assert_rect(&pieces[1].crop, (4000.0, 0.0, 1920.0 / cover, 1000.0));
        // Centred: as much is cut from the right as from the left.
        let right_edge = pieces[1].crop.x + pieces[1].crop.width;
        assert!((8000.0 - right_edge - hidden_each_side).abs() < 1e-6);
    }

    #[test]
    fn a_picture_taller_than_the_canvas_loses_top_and_bottom_evenly() {
        let displays = [display("left", 0.0, 0.0, 1920.0, 1080.0, 1.0), display("right", 1920.0, 0.0, 1920.0, 1080.0, 1.0)];
        // Width decides the scale: 3840 / 1000 points per pixel.
        let pieces = span_layout(1000, 2000, &displays);
        assert_sane((1000, 2000), &displays, &pieces);
        let cover = 3.84;
        let visible_height = 1080.0 / cover;
        let hidden_above = (2000.0 - visible_height) / 2.0;
        assert_rect(&pieces[0].crop, (0.0, hidden_above, 500.0, visible_height));
        assert_rect(&pieces[1].crop, (500.0, hidden_above, 500.0, visible_height));
    }

    #[test]
    fn staggered_displays_span_their_bounding_box() {
        // Different sizes, the second one lower and to the right, with a
        // corner of the canvas that belongs to no display.
        let displays = [display("a", 0.0, 0.0, 1440.0, 900.0, 2.0), display("b", 1440.0, 300.0, 1920.0, 1200.0, 1.0)];
        let canvas = canvas_bounds(&displays).unwrap();
        assert_rect(&canvas, (0.0, 0.0, 3360.0, 1500.0));
        let pieces = span_layout(3360, 1500, &displays);
        assert_sane((3360, 1500), &displays, &pieces);
        assert_rect(&pieces[0].crop, (0.0, 0.0, 1440.0, 900.0));
        assert_rect(&pieces[1].crop, (1440.0, 300.0, 1920.0, 1200.0));
    }

    #[test]
    fn a_single_display_is_an_ordinary_centred_fill() {
        let displays = [display("only", 0.0, 0.0, 1512.0, 982.0, 2.0)];
        let pieces = span_layout(6000, 3000, &displays);
        assert_sane((6000, 3000), &displays, &pieces);
        let cover = 982.0 / 3000.0;
        let visible_width = 1512.0 / cover;
        assert_rect(&pieces[0].crop, ((6000.0 - visible_width) / 2.0, 0.0, visible_width, 3000.0));
        assert_eq!((pieces[0].out_width, pieces[0].out_height), (3024, 1964));
    }

    #[test]
    fn nothing_to_lay_out_gives_no_pieces() {
        assert!(span_layout(100, 100, &[]).is_empty());
        assert!(span_layout(0, 100, &[display("a", 0.0, 0.0, 100.0, 100.0, 1.0)]).is_empty());
        assert!(canvas_bounds(&[]).is_none());
    }

    #[test]
    fn neighbouring_crops_share_their_edge_after_snapping_to_pixels() {
        let displays = [
            display("a", 0.0, 0.0, 1512.0, 982.0, 2.0),
            display("b", 1512.0, 0.0, 2560.0, 1440.0, 1.0),
            display("c", 4072.0, 0.0, 1080.0, 1920.0, 1.0),
        ];
        // An awkward size so every crop edge is fractional.
        let (width, height) = (5003, 2711);
        let pieces = span_layout(width, height, &displays);
        assert_sane((width, height), &displays, &pieces);
        let snapped: Vec<_> = pieces.iter().map(|piece| pixel_crop(&piece.crop, width, height)).collect();
        for pair in snapped.windows(2) {
            assert_eq!(pair[0].0 + pair[0].2, pair[1].0, "no gap and no overlap between neighbours");
        }
        for (x, y, w, h) in snapped {
            assert!(w >= 1 && h >= 1);
            assert!(x + w <= width && y + h <= height);
        }
    }

    #[test]
    fn pixel_crop_never_leaves_the_picture_or_collapses() {
        assert_eq!(pixel_crop(&Rect { x: -3.0, y: -0.4, width: 50.0, height: 20.0 }, 40, 10), (0, 0, 40, 10));
        assert_eq!(pixel_crop(&Rect { x: 39.9, y: 9.9, width: 0.01, height: 0.01 }, 40, 10), (39, 9, 1, 1));
        assert_eq!(pixel_crop(&Rect { x: 10.4, y: 2.6, width: 5.2, height: 3.1 }, 40, 10), (10, 3, 6, 3));
    }

    #[test]
    fn rendering_writes_one_correctly_sized_piece_per_display_and_reuses_them() {
        let dir = std::env::temp_dir().join(format!("still-span-test-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        let source = dir.join("source.png");
        // Left half red, right half blue, so each piece is recognisable.
        let picture = RgbImage::from_fn(400, 100, |x, _| if x < 200 { image::Rgb([220, 20, 20]) } else { image::Rgb([20, 20, 220]) });
        picture.save(&source).unwrap();

        let displays = [display("1", 0.0, 0.0, 100.0, 50.0, 2.0), display("2", 100.0, 0.0, 100.0, 50.0, 1.0)];
        let cache = dir.join("span");
        let pieces = render_span(&source, "item", &displays, &cache).unwrap();
        assert_eq!(pieces.len(), 2);

        let left = image::open(&pieces[0].1).unwrap().into_rgb8();
        let right = image::open(&pieces[1].1).unwrap().into_rgb8();
        assert_eq!((left.width(), left.height()), (200, 100));
        assert_eq!((right.width(), right.height()), (100, 50));
        let centre = |picture: &RgbImage| *picture.get_pixel(picture.width() / 2, picture.height() / 2);
        assert!(centre(&left)[0] > 180 && centre(&left)[2] < 80, "left piece should be red");
        assert!(centre(&right)[2] > 180 && centre(&right)[0] < 80, "right piece should be blue");

        // Same inputs: same files, untouched.
        let before = fs::metadata(&pieces[0].1).unwrap().modified().unwrap();
        let again = render_span(&source, "item", &displays, &cache).unwrap();
        assert_eq!(again, pieces);
        assert_eq!(fs::metadata(&again[0].1).unwrap().modified().unwrap(), before);

        // A different arrangement gets different file names, and pruning keeps only those.
        let moved = [display("1", 0.0, 0.0, 100.0, 50.0, 2.0), display("2", 0.0, 50.0, 100.0, 50.0, 1.0)];
        let rearranged = render_span(&source, "item", &moved, &cache).unwrap();
        assert_ne!(rearranged[0].1, pieces[0].1);
        let keep: Vec<PathBuf> = rearranged.iter().map(|(_, path)| path.clone()).collect();
        prune_span_cache(&cache, &keep);
        let left_over = fs::read_dir(&cache).unwrap().count();
        assert_eq!(left_over, 2);

        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn spanning_refuses_files_that_are_not_pictures_it_knows() {
        let dir = std::env::temp_dir().join(format!("still-span-bad-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        let displays = [display("1", 0.0, 0.0, 100.0, 50.0, 1.0)];

        let gif = dir.join("anim.gif");
        fs::write(&gif, b"GIF89a\x01\x00\x01\x00\x00\x00\x00;").unwrap();
        let error = render_span(&gif, "gif", &displays, &dir.join("span")).unwrap_err();
        assert!(error.contains("JPEG, PNG and WebP"), "{error}");

        let missing = render_span(&dir.join("nope.jpg"), "nope", &displays, &dir.join("span")).unwrap_err();
        assert_eq!(missing, "That file is not in the library.");
        let _ = fs::remove_dir_all(&dir);
    }
}
