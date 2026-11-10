fn main() {
    // Naming the commands here makes Tauri generate an `allow-<command>`
    // permission for each one and deny them to every window by default. Only
    // capabilities/main.json grants them, and only to the main window, so the
    // live wallpaper windows cannot call into the app.
    let commands = &[
        "displays_list",
        "library_store",
        "library_download",
        "library_remove",
        "library_resolve",
        "wallpaper_apply",
        "rotation_sync",
        "rotation_command",
        "rotation_status",
    ];
    let attributes =
        tauri_build::Attributes::new().app_manifest(tauri_build::AppManifest::new().commands(commands));
    if let Err(error) = tauri_build::try_build(attributes) {
        panic!("tauri-build failed: {error:#}");
    }
}
