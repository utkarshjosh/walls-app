//! Live wallpapers: one borderless web view per display, sitting just above
//! the desktop picture and below Finder's icons.
//!
//! A video is played by the app's own `live.html`; a scene is a self-contained
//! HTML file loaded straight from the library. Both learn where they sit from
//! the URL fragment (see `LiveViewport` in the UI's `platform/types.ts`), so
//! one wallpaper can run seamlessly across several displays.
//!
//! The windows are transparent. Until a wallpaper has something to draw, and
//! if it fails to load at all, the ordinary desktop picture underneath (the
//! wallpaper's poster) shows through, so the desktop is never left blank.
//!
//! These windows are deliberately powerless: they are not listed in the
//! capability file and every command refuses callers that are not the main
//! window, so nothing a wallpaper runs can reach the native side.

use std::collections::HashMap;
use std::time::{Duration, Instant};

use objc2::MainThreadMarker;
use objc2_app_kit::{NSWindow, NSWindowAnimationBehavior, NSWindowCollectionBehavior};
use tauri::{AppHandle, Manager, Url, WebviewUrl, WebviewWindow, WebviewWindowBuilder};

use crate::displays;
use crate::main_thread::on_main;
use crate::model::{DisplayInfo, PlayItem, WallpaperKind};

#[link(name = "CoreGraphics", kind = "framework")]
extern "C" {
    fn CGWindowLevelForKey(key: i32) -> i32;
}

/// `kCGDesktopWindowLevelKey`: the level of the desktop picture itself.
/// Finder's icons live 20 levels higher (`kCGDesktopIconWindowLevelKey`).
const DESKTOP_WINDOW_LEVEL_KEY: i32 = 2;

/// Where one live window sits inside the surface its wallpaper covers, in points.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Viewport {
    pub canvas_width: f64,
    pub canvas_height: f64,
    pub x: f64,
    pub y: f64,
}

#[derive(Debug, Clone, PartialEq)]
pub struct LiveTarget {
    pub display: DisplayInfo,
    pub item: PlayItem,
    pub viewport: Viewport,
}

struct Shown {
    /// The page without its fragment; a different page means a different wallpaper.
    page: String,
    video_source: Option<String>,
    viewport: Viewport,
    paused: bool,
    rect: [f64; 4],
}

/// The live windows currently open, keyed by display id. Owned by the
/// rotation worker; never touched from the main thread.
#[derive(Default)]
pub struct LiveWindows {
    shown: HashMap<String, Shown>,
}

impl LiveWindows {
    pub fn is_empty(&self) -> bool {
        self.shown.is_empty()
    }

    /// Make the open live windows match `wanted`.
    ///
    /// `scope` lists the displays this change covers: a display in scope that
    /// is not wanted loses its live window (it now shows a still picture).
    /// Displays outside the scope are left alone, unless they are no longer
    /// in `connected`.
    pub fn sync(
        &mut self,
        app: &AppHandle,
        wanted: &[LiveTarget],
        scope: &[String],
        connected: &[String],
        paused: bool,
    ) -> Result<(), String> {
        if crate::dry_run() {
            for target in wanted {
                eprintln!(
                    "still (dry run): would play {} on display {}",
                    target.item.path, target.display.id
                );
            }
            return Ok(());
        }

        let is_wanted = |id: &String| wanted.iter().any(|target| target.display.id == *id);
        let stale: Vec<String> = self
            .shown
            .keys()
            .filter(|id| !connected.contains(id) || (scope.contains(id) && !is_wanted(id)))
            .cloned()
            .collect();
        for id in stale {
            self.close(app, &id);
        }

        let mut first_error = None;
        for target in wanted {
            if let Err(error) = self.ensure(app, target, paused) {
                first_error.get_or_insert(error);
            }
        }
        first_error.map_or(Ok(()), Err)
    }

    /// Freeze or resume every live wallpaper (used when the Mac goes on or off battery).
    pub fn set_paused(&mut self, app: &AppHandle, paused: bool) {
        for (display_id, shown) in &mut self.shown {
            if shown.paused == paused {
                continue;
            }
            shown.paused = paused;
            if let Some(window) = app.get_webview_window(&label_for(display_id)) {
                set_fragment(&window, &fragment(&shown.viewport, paused, shown.video_source.as_deref()));
            }
        }
    }

    fn close(&mut self, app: &AppHandle, display_id: &str) {
        self.shown.remove(display_id);
        if let Some(window) = app.get_webview_window(&label_for(display_id)) {
            let _ = window.destroy();
        }
    }

    fn ensure(&mut self, app: &AppHandle, target: &LiveTarget, paused: bool) -> Result<(), String> {
        let display_id = target.display.id.clone();
        let label = label_for(&display_id);
        let (page, video_source) = page_for(&app_base(app), &target.item);
        let hash = fragment(&target.viewport, paused, video_source.as_deref());
        let rect = [target.display.x, target.display.y, target.display.width, target.display.height];
        let url = Url::parse(&format!("{page}#{hash}"))
            .map_err(|_| "That live wallpaper has a file name Still cannot open.".to_string())?;

        let existing = app.get_webview_window(&label);
        match (existing, self.shown.get(&display_id)) {
            (Some(window), Some(shown)) if shown.page == page => {
                if shown.paused != paused || shown.viewport != target.viewport {
                    set_fragment(&window, &hash);
                }
                if shown.rect != rect {
                    place(app, &window, &display_id)?;
                }
            }
            (Some(window), Some(_)) => {
                // A different wallpaper for this display: reuse the window.
                window
                    .navigate(url)
                    .map_err(|error| format!("The live wallpaper could not be loaded: {error}."))?;
                place(app, &window, &display_id)?;
            }
            (leftover, _) => {
                // A window we are not tracking can only be one that is still
                // closing; its label must be free before it can be reused.
                if let Some(window) = leftover {
                    let _ = window.destroy();
                }
                wait_until_gone(app, &label);
                let window = create(app, &label, url, &target.display)?;
                if let Err(error) = place(app, &window, &display_id) {
                    let _ = window.destroy();
                    return Err(error);
                }
            }
        }
        self.shown.insert(display_id, Shown { page, video_source, viewport: target.viewport, paused, rect });
        Ok(())
    }
}

fn create(app: &AppHandle, label: &str, url: Url, display: &DisplayInfo) -> Result<WebviewWindow, String> {
    let base = app_base(app);
    let webview_url = if matches!(url.scheme(), "http" | "https") {
        WebviewUrl::External(url)
    } else {
        WebviewUrl::CustomProtocol(url)
    };
    // Created hidden, because a new window starts at the normal level, above
    // other apps; `place` moves it down to the desktop and only then shows it.
    WebviewWindowBuilder::new(app, label, webview_url)
        .title("Still Live Wallpaper")
        .decorations(false)
        .shadow(false)
        .resizable(false)
        .maximizable(false)
        .minimizable(false)
        .closable(false)
        .focused(false)
        .focusable(false)
        .visible(false)
        .skip_taskbar(true)
        .accept_first_mouse(false)
        .position(display.x, display.y)
        .inner_size(display.width, display.height)
        .transparent(true)
        .disable_drag_drop_handler()
        // A wallpaper stays where it was put: a scene cannot wander off to a website.
        .on_navigation(move |destination| stays_local(&base, destination))
        .build()
        .map_err(|error| format!("The live wallpaper window could not be opened: {error}."))
}

/// Put the window exactly over its display at desktop level, and show it.
fn place(app: &AppHandle, window: &WebviewWindow, display_id: &str) -> Result<(), String> {
    let window = window.clone();
    let display_id = display_id.to_string();
    on_main(app, move |mtm| {
        let Some(screen) = displays::find_screen(mtm, &display_id) else {
            return Err("That display is no longer connected.".to_string());
        };
        // The screen's own frame, in AppKit coordinates: no conversion, no rounding.
        let frame = screen.frame();
        with_ns_window(&window, mtm, |ns_window| {
            // SAFETY: a plain CoreGraphics lookup with a documented key.
            let desktop_level = unsafe { CGWindowLevelForKey(DESKTOP_WINDOW_LEVEL_KEY) };
            ns_window.setLevel(desktop_level as isize);
            // On every Space, fixed in place during Mission Control, and
            // skipped by Cmd-` and the Window menu.
            ns_window.setCollectionBehavior(
                NSWindowCollectionBehavior::CanJoinAllSpaces
                    | NSWindowCollectionBehavior::Stationary
                    | NSWindowCollectionBehavior::IgnoresCycle,
            );
            ns_window.setIgnoresMouseEvents(true);
            ns_window.setHasShadow(false);
            ns_window.setExcludedFromWindowsMenu(true);
            // Hiding the app (Cmd-H) must not take the wallpaper with it.
            ns_window.setCanHide(false);
            ns_window.setHidesOnDeactivate(false);
            ns_window.setAnimationBehavior(NSWindowAnimationBehavior::None);
            ns_window.setFrame_display(frame, true);
            // "Regardless": show it without making Still the active app.
            ns_window.orderFrontRegardless();
        })
        .ok_or_else(|| "The live wallpaper window is gone.".to_string())
    })?
}

fn with_ns_window<R>(window: &WebviewWindow, _mtm: MainThreadMarker, work: impl FnOnce(&NSWindow) -> R) -> Option<R> {
    let pointer = window.ns_window().ok()?.cast::<NSWindow>();
    // SAFETY: Tauri returns the live NSWindow behind this window; we are on the
    // main thread (the marker proves it) and the reference ends with this call.
    let ns_window = unsafe { pointer.as_ref() }?;
    Some(work(ns_window))
}

fn set_fragment(window: &WebviewWindow, hash: &str) {
    // JSON string syntax is also valid JavaScript string syntax.
    if let Ok(literal) = serde_json::to_string(hash) {
        let _ = window.eval(format!("location.hash = {literal};"));
    }
}

/// Closing is finished by the event loop a moment after it is asked for, and
/// the label cannot be reused until then.
fn wait_until_gone(app: &AppHandle, label: &str) {
    let deadline = Instant::now() + Duration::from_secs(2);
    while app.get_webview_window(label).is_some() && Instant::now() < deadline {
        std::thread::sleep(Duration::from_millis(15));
    }
}

/// Where the app's own pages are served from: the Vite dev server while
/// developing, the built-in `tauri://localhost` in a real build.
fn app_base(app: &AppHandle) -> String {
    if tauri::is_dev() {
        if let Some(dev_url) = &app.config().build.dev_url {
            return dev_url.to_string();
        }
    }
    "tauri://localhost/".to_string()
}

pub fn label_for(display_id: &str) -> String {
    format!("live-{display_id}")
}

/// The page that plays `item` (without a fragment) and, for a video, the URL
/// of the video file that page should play.
pub fn page_for(app_base: &str, item: &PlayItem) -> (String, Option<String>) {
    let file = asset_url(&item.path);
    if item.kind == WallpaperKind::Video {
        let base = app_base.trim_end_matches('/');
        (format!("{base}/live.html?src={}", encode_uri_component(&file)), Some(file))
    } else {
        (file, None)
    }
}

/// `#cw=..&ch=..&vx=..&vy=..&paused=0|1`, plus `&src=` for a video.
///
/// The video address travels both in the query (see `page_for`) and here: the
/// UI contract describes it as an extra fragment field, and keeping it in the
/// fragment means a later pause/resume update does not drop it.
pub fn fragment(viewport: &Viewport, paused: bool, video_source: Option<&str>) -> String {
    let mut hash = format!(
        "cw={}&ch={}&vx={}&vy={}&paused={}",
        viewport.canvas_width,
        viewport.canvas_height,
        viewport.x,
        viewport.y,
        u8::from(paused)
    );
    if let Some(source) = video_source {
        hash.push_str("&src=");
        hash.push_str(&encode_uri_component(source));
    }
    hash
}

/// The URL the asset protocol serves a library file under; the same string
/// the UI gets from `convertFileSrc(path)` on macOS.
pub fn asset_url(path: &str) -> String {
    format!("asset://localhost/{}", encode_uri_component(path))
}

/// JavaScript's `encodeURIComponent`.
pub fn encode_uri_component(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    for byte in text.bytes() {
        let unreserved = byte.is_ascii_alphanumeric() || b"-_.!~*'()".contains(&byte);
        if unreserved {
            out.push(byte as char);
        } else {
            out.push_str(&format!("%{byte:02X}"));
        }
    }
    out
}

/// Navigation allowed inside a live window: library files and the app's own pages.
fn stays_local(app_base: &str, destination: &Url) -> bool {
    if matches!(destination.scheme(), "asset" | "about") {
        return true;
    }
    Url::parse(app_base).is_ok_and(|base| {
        base.scheme() == destination.scheme()
            && base.host_str() == destination.host_str()
            && base.port_or_known_default() == destination.port_or_known_default()
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn item(kind: WallpaperKind, path: &str) -> PlayItem {
        PlayItem { id: "x".into(), kind, path: path.into(), width: 0.0, height: 0.0, poster_path: None }
    }

    const LIBRARY_FILE: &str = "/Users/me/Library/Application Support/com.utkarshjoshi.still/library/clip-1.mp4";

    #[test]
    fn encoding_matches_encode_uri_component() {
        assert_eq!(encode_uri_component("abc-_.!~*'()XYZ019"), "abc-_.!~*'()XYZ019");
        assert_eq!(encode_uri_component("/a b/c.mp4"), "%2Fa%20b%2Fc.mp4");
        assert_eq!(encode_uri_component("a&b=c#d?e:f"), "a%26b%3Dc%23d%3Fe%3Af");
        assert_eq!(encode_uri_component("caf\u{e9}"), "caf%C3%A9");
    }

    #[test]
    fn asset_urls_match_convert_file_src() {
        assert_eq!(
            asset_url(LIBRARY_FILE),
            "asset://localhost/%2FUsers%2Fme%2FLibrary%2FApplication%20Support%2Fcom.utkarshjoshi.still%2Flibrary%2Fclip-1.mp4"
        );
    }

    #[test]
    fn a_scene_is_loaded_directly_from_the_library() {
        let (page, source) = page_for("tauri://localhost/", &item(WallpaperKind::Scene, "/lib/aurora.html"));
        assert_eq!(page, "asset://localhost/%2Flib%2Faurora.html");
        assert_eq!(source, None);
    }

    #[test]
    fn a_video_is_played_by_the_bundled_page_in_dev_and_production() {
        let video = item(WallpaperKind::Video, "/lib/clip.mp4");
        let (page, source) = page_for("tauri://localhost/", &video);
        assert_eq!(source.as_deref(), Some("asset://localhost/%2Flib%2Fclip.mp4"));
        assert_eq!(page, "tauri://localhost/live.html?src=asset%3A%2F%2Flocalhost%2F%252Flib%252Fclip.mp4");
        let (dev_page, _) = page_for("http://localhost:5173/", &video);
        assert!(dev_page.starts_with("http://localhost:5173/live.html?src=asset%3A%2F%2F"));

        // The page gets the file's address back, exactly, from the query string.
        let parsed = Url::parse(&page).unwrap();
        let (_, value) = parsed.query_pairs().find(|(key, _)| key == "src").unwrap();
        assert_eq!(value, "asset://localhost/%2Flib%2Fclip.mp4");
    }

    #[test]
    fn the_fragment_carries_geometry_and_pause_state() {
        let own_screen = Viewport { canvas_width: 1512.0, canvas_height: 982.0, x: 0.0, y: 0.0 };
        assert_eq!(fragment(&own_screen, false, None), "cw=1512&ch=982&vx=0&vy=0&paused=0");
        let spanned = Viewport { canvas_width: 4072.0, canvas_height: 1440.0, x: 1512.0, y: 229.5 };
        assert_eq!(fragment(&spanned, true, None), "cw=4072&ch=1440&vx=1512&vy=229.5&paused=1");
        assert_eq!(
            fragment(&own_screen, false, Some("asset://localhost/%2Flib%2Fclip.mp4")),
            "cw=1512&ch=982&vx=0&vy=0&paused=0&src=asset%3A%2F%2Flocalhost%2F%252Flib%252Fclip.mp4"
        );
    }

    #[test]
    fn a_whole_live_url_parses_and_keeps_its_parts() {
        let video = item(WallpaperKind::Video, LIBRARY_FILE);
        let (page, source) = page_for("http://localhost:5173/", &video);
        let viewport = Viewport { canvas_width: 3024.0, canvas_height: 982.0, x: 1512.0, y: 0.0 };
        let url = Url::parse(&format!("{page}#{}", fragment(&viewport, false, source.as_deref()))).unwrap();
        assert_eq!(url.path(), "/live.html");
        let hash = url.fragment().unwrap();
        assert!(hash.starts_with("cw=3024&ch=982&vx=1512&vy=0&paused=0&src="));
    }

    #[test]
    fn labels_follow_the_display_id() {
        assert_eq!(label_for("69734662"), "live-69734662");
    }

    #[test]
    fn live_windows_may_only_stay_on_local_pages() {
        let allowed = |base: &str, to: &str| stays_local(base, &Url::parse(to).unwrap());
        assert!(allowed("tauri://localhost/", "asset://localhost/%2Flib%2Fscene.html#cw=1"));
        assert!(allowed("tauri://localhost/", "tauri://localhost/live.html?src=x"));
        assert!(allowed("http://localhost:5173/", "http://localhost:5173/live.html"));
        assert!(allowed("tauri://localhost/", "about:blank"));
        assert!(!allowed("tauri://localhost/", "https://example.com/"));
        assert!(!allowed("http://localhost:5173/", "http://localhost:8080/"));
        assert!(!allowed("http://localhost:5173/", "http://evil.example:5173/"));
        assert!(!allowed("tauri://localhost/", "file:///etc/passwd"));
    }

    #[test]
    fn the_desktop_level_sits_below_icons_and_ordinary_windows() {
        // Read-only lookups; nothing is created.
        let (desktop, icons, normal) =
            unsafe { (CGWindowLevelForKey(DESKTOP_WINDOW_LEVEL_KEY), CGWindowLevelForKey(18), CGWindowLevelForKey(4)) };
        assert!(desktop < icons, "desktop {desktop} should be below desktop icons {icons}");
        assert!(icons < normal, "desktop icons {icons} should be below normal windows {normal}");
        assert_eq!(normal, 0);
    }
}
