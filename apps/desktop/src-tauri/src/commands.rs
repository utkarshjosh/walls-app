//! The commands the UI calls. Names, argument names and return shapes are the
//! contract in `apps/web/src/platform/types.ts` (see `desktop.ts` for the calls).
//!
//! Every command is `async`, which in Tauri means it runs off the main thread;
//! anything slow (files, network, waiting for the rotation worker) is pushed
//! further onto a blocking thread. Every command also checks who is calling:
//! the capability file already limits commands to the main window, and this
//! second check means a live wallpaper window is refused even if that file is
//! ever loosened by mistake.

use std::collections::BTreeMap;

use tauri::ipc::{InvokeBody, Request};
use tauri::{AppHandle, State, Webview};

use crate::library::{self, Library};
use crate::model::{ApplyRequest, DisplayInfo, RotationCommand, RotationPlan, RotationStatus, StoredFile};
use crate::rotation::Rotation;
use crate::{displays, MAIN_WINDOW};

fn only_main_window(webview: &Webview) -> Result<(), String> {
    if webview.label() == MAIN_WINDOW {
        Ok(())
    } else {
        Err("This window is not allowed to control Still.".to_string())
    }
}

async fn blocking<T, F>(work: F) -> Result<T, String>
where
    T: Send + 'static,
    F: FnOnce() -> Result<T, String> + Send + 'static,
{
    tauri::async_runtime::spawn_blocking(work)
        .await
        .map_err(|_| "Still ran into an internal problem.".to_string())?
}

#[tauri::command]
pub async fn displays_list(webview: Webview, app: AppHandle) -> Result<Vec<DisplayInfo>, String> {
    only_main_window(&webview)?;
    blocking(move || displays::fetch(&app)).await
}

/// The file's bytes are the raw request body; its name travels URL-encoded in
/// the `x-file-name` header.
#[tauri::command]
pub async fn library_store(
    webview: Webview,
    library: State<'_, Library>,
    request: Request<'_>,
) -> Result<StoredFile, String> {
    only_main_window(&webview)?;
    let name = request
        .headers()
        .get("x-file-name")
        .and_then(|value| value.to_str().ok())
        .and_then(library::decode_header_name)
        .ok_or_else(|| "The file arrived without a usable name.".to_string())?;
    let InvokeBody::Raw(bytes) = request.body() else {
        return Err("The file's contents did not arrive.".to_string());
    };
    // Written right here rather than on a blocking thread: the body is borrowed
    // from the request, and copying a large video just to move it would cost
    // more than the write. This is a background thread, not the main one.
    library.store(&name, bytes)
}

#[tauri::command]
pub async fn library_download(
    webview: Webview,
    library: State<'_, Library>,
    name: String,
    url: String,
) -> Result<StoredFile, String> {
    only_main_window(&webview)?;
    let library = library.inner().clone();
    blocking(move || library.download(&name, &url)).await
}

#[tauri::command]
pub async fn library_remove(
    webview: Webview,
    library: State<'_, Library>,
    names: Vec<String>,
) -> Result<(), String> {
    only_main_window(&webview)?;
    let library = library.inner().clone();
    blocking(move || library.remove(&names)).await
}

#[tauri::command]
pub async fn library_resolve(
    webview: Webview,
    library: State<'_, Library>,
    names: Vec<String>,
) -> Result<BTreeMap<String, StoredFile>, String> {
    only_main_window(&webview)?;
    let library = library.inner().clone();
    blocking(move || Ok(library.resolve(&names))).await
}

#[tauri::command]
pub async fn wallpaper_apply(
    webview: Webview,
    rotation: State<'_, Rotation>,
    request: ApplyRequest,
) -> Result<RotationStatus, String> {
    only_main_window(&webview)?;
    let rotation = rotation.inner().clone();
    blocking(move || rotation.apply(request)).await
}

#[tauri::command]
pub async fn rotation_sync(
    webview: Webview,
    rotation: State<'_, Rotation>,
    plan: RotationPlan,
) -> Result<RotationStatus, String> {
    only_main_window(&webview)?;
    let rotation = rotation.inner().clone();
    blocking(move || rotation.sync(plan)).await
}

#[tauri::command]
pub async fn rotation_command(
    webview: Webview,
    rotation: State<'_, Rotation>,
    command: RotationCommand,
) -> Result<RotationStatus, String> {
    only_main_window(&webview)?;
    let rotation = rotation.inner().clone();
    blocking(move || rotation.command(command)).await
}

#[tauri::command]
pub async fn rotation_status(webview: Webview, rotation: State<'_, Rotation>) -> Result<RotationStatus, String> {
    only_main_window(&webview)?;
    Ok(rotation.status())
}
