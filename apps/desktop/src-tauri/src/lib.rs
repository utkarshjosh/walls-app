//! Still's native core: the Tauri shell around the shared React UI.
//!
//! The UI talks to this crate only through the commands in `commands.rs` and
//! two events; the contract lives in `apps/web/src/platform/types.ts`.
//!
//!   model      the contract's types
//!   engine     which wallpaper comes next            (pure)
//!   schedule   when the next change is due           (pure)
//!   library    the folder of wallpaper files
//!   displays   the attached displays, via AppKit
//!   wallpaper  span geometry (pure) and setting desktop pictures
//!   live       desktop-level windows for video and scene wallpapers
//!   power      battery or mains
//!   rotation   the worker that owns state, timer and persistence
//!   tray       the menu-bar item
//!   commands   what the UI can call

mod commands;
mod displays;
mod engine;
mod library;
mod live;
mod main_thread;
mod model;
mod power;
mod rotation;
mod schedule;
mod tray;
mod wallpaper;

use std::sync::OnceLock;

use objc2::MainThreadMarker;
use tauri::{AppHandle, Manager, RunEvent, WindowEvent};

use library::Library;
use rotation::{Rotation, WorkerSetup};

/// The only window allowed to call commands, and the only one events go to.
pub(crate) const MAIN_WINDOW: &str = "main";

/// `STILL_DRY_RUN=1` makes the app log what it would put on the desktop
/// instead of doing it: no desktop picture is set and no live window opens.
/// For trying the UI and the rotation logic without touching the real desktop.
pub(crate) fn dry_run() -> bool {
    static DRY_RUN: OnceLock<bool> = OnceLock::new();
    *DRY_RUN.get_or_init(|| std::env::var("STILL_DRY_RUN").is_ok_and(|value| value == "1"))
}

pub fn run() {
    if std::env::args().any(|argument| argument == "--print-displays") {
        print_displays();
        return;
    }

    let built = tauri::Builder::default()
        // Registered first so a second copy exits before it starts a second
        // timer over the same saved state; the running copy shows its window.
        .plugin(tauri_plugin_single_instance::init(|app, _arguments, _directory| show_main_window(app)))
        .invoke_handler(tauri::generate_handler![
            commands::displays_list,
            commands::library_store,
            commands::library_download,
            commands::library_remove,
            commands::library_resolve,
            commands::wallpaper_apply,
            commands::rotation_sync,
            commands::rotation_command,
            commands::rotation_status,
        ])
        .on_window_event(|window, event| {
            // Closing the window only hides it: rotation and live wallpapers
            // keep running. Quit (Cmd-Q or the tray) is what ends the app.
            if window.label() == MAIN_WINDOW {
                if let WindowEvent::CloseRequested { api, .. } = event {
                    api.prevent_close();
                    let _ = window.hide();
                }
            }
        })
        .setup(|app| {
            start(app.handle())?;
            Ok(())
        })
        .build(tauri::generate_context!());

    match built {
        Ok(app) => app.run(|app, event| match event {
            // Clicking the Dock icon. Always show the main window: live
            // wallpaper windows count as "visible windows" but are not it.
            RunEvent::Reopen { .. } => show_main_window(app),
            // `code` is `None` when Tauri wants to exit only because the last
            // window went away; a real quit carries an exit code.
            RunEvent::ExitRequested { code: None, api, .. } => api.prevent_exit(),
            _ => {}
        }),
        Err(error) => {
            eprintln!("Still could not start: {error}");
            std::process::exit(1);
        }
    }
}

fn start(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    let data_dir = app.path().app_data_dir()?;
    let cache_dir = app.path().app_cache_dir()?;
    // These two folders are also the only ones the asset protocol serves
    // (see `assetProtocol.scope` in tauri.conf.json).
    let library = Library::open(data_dir.join("library"))?;

    let (rotation, inbox) = Rotation::channel();
    let tray = tray::build(app, rotation.clone())?;
    rotation::spawn(
        &rotation,
        inbox,
        WorkerSetup {
            app: app.clone(),
            library: library.clone(),
            state_file: data_dir.join("rotation.json"),
            span_cache: cache_dir.join("span"),
            tray,
        },
    )?;

    app.manage(library);
    app.manage(rotation.clone());

    // Setup runs on the main thread, which is where AppKit wants observers added.
    if let Some(mtm) = MainThreadMarker::new() {
        displays::watch(mtm, move || rotation.displays_changed());
    }
    Ok(())
}

pub(crate) fn show_main_window(app: &AppHandle) {
    if let Some(window) = app.get_webview_window(MAIN_WINDOW) {
        let _ = window.unminimize();
        let _ = window.show();
        let _ = window.set_focus();
    }
}

/// `still --print-displays`: list the displays as the UI would receive them
/// and exit. Read-only; handy for checking coordinates on a new arrangement.
fn print_displays() {
    let Some(mtm) = MainThreadMarker::new() else {
        eprintln!("Displays can only be read from the main thread.");
        std::process::exit(1);
    };
    match serde_json::to_string_pretty(&displays::list(mtm)) {
        Ok(json) => println!("{json}"),
        Err(error) => {
            eprintln!("The displays could not be listed: {error}");
            std::process::exit(1);
        }
    }
}
