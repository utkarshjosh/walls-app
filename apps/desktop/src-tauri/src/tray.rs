//! The menu-bar item: change, pause and reopen without the window.
//!
//! Menu handlers run on the main thread, so they only post to the rotation
//! worker; the worker calls `sync` back when the status changes.

use tauri::menu::{Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::{TrayIcon, TrayIconBuilder};
use tauri::AppHandle;

use crate::model::{RotationCommand, RotationStatus};
use crate::rotation::Rotation;

const NEXT: &str = "next";
const PREVIOUS: &str = "previous";
const PAUSE_OR_RESUME: &str = "pause-or-resume";
const OPEN: &str = "open";
const QUIT: &str = "quit";

pub struct Tray {
    previous: MenuItem<tauri::Wry>,
    pause_or_resume: MenuItem<tauri::Wry>,
    // Held because dropping the icon would remove it from the menu bar.
    icon: TrayIcon<tauri::Wry>,
}

pub fn build(app: &AppHandle, rotation: Rotation) -> tauri::Result<Tray> {
    let next = MenuItem::with_id(app, NEXT, "Next Wallpaper", true, None::<&str>)?;
    // Disabled until there is something to go back to.
    let previous = MenuItem::with_id(app, PREVIOUS, "Previous Wallpaper", false, None::<&str>)?;
    let pause_or_resume = MenuItem::with_id(app, PAUSE_OR_RESUME, "Pause Rotation", true, None::<&str>)?;
    let open = MenuItem::with_id(app, OPEN, "Open Still", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, QUIT, "Quit Still", true, None::<&str>)?;
    let separator = PredefinedMenuItem::separator(app)?;
    let menu = Menu::with_items(app, &[&next, &previous, &pause_or_resume, &separator, &open, &quit])?;

    let icon = TrayIconBuilder::with_id("still")
        // Black-and-alpha artwork; as a template macOS recolours it for light
        // and dark menu bars. Regenerate with `npm run icons`.
        .icon(tauri::include_image!("icons/tray-template.png"))
        .icon_as_template(true)
        .tooltip("Still")
        .menu(&menu)
        .show_menu_on_left_click(true)
        .on_menu_event(move |app, event| match event.id().as_ref() {
            NEXT => rotation.post(RotationCommand::Next),
            PREVIOUS => rotation.post(RotationCommand::Previous),
            PAUSE_OR_RESUME => {
                let command = if rotation.status().paused { RotationCommand::Resume } else { RotationCommand::Pause };
                rotation.post(command);
            }
            OPEN => crate::show_main_window(app),
            QUIT => app.exit(0),
            _ => {}
        })
        .build(app)?;

    Ok(Tray { previous, pause_or_resume, icon })
}

impl Tray {
    /// Make the menu reflect `status`. Safe from any thread: Tauri applies
    /// menu changes on the main thread itself.
    pub fn sync(&self, status: &RotationStatus) {
        let _ = self.previous.set_enabled(status.can_go_previous);
        let label = if status.paused { "Resume Rotation" } else { "Pause Rotation" };
        let _ = self.pause_or_resume.set_text(label);
        if crate::dry_run() {
            eprintln!(
                "still (dry run): menu-bar item at {:?} shows {:?}, Previous Wallpaper enabled: {:?}",
                self.icon.rect().ok().flatten(),
                self.pause_or_resume.text().ok(),
                self.previous.is_enabled().ok()
            );
        }
    }
}
