//! AppKit may only be touched from the main thread. Everything else in the app
//! (commands, the rotation worker) runs on background threads and hops over
//! with `on_main`, waiting for the answer.
//!
//! The one rule that keeps this deadlock-free: the main thread never waits on
//! a background thread. Menu, window and notification handlers only post
//! messages; they never block for a reply.

use objc2::MainThreadMarker;
use tauri::AppHandle;

/// Run `work` on the main thread and return its result.
pub fn on_main<T, F>(app: &AppHandle, work: F) -> Result<T, String>
where
    T: Send + 'static,
    F: FnOnce(MainThreadMarker) -> T + Send + 'static,
{
    let (sender, receiver) = std::sync::mpsc::sync_channel(1);
    app.run_on_main_thread(move || {
        if let Some(marker) = MainThreadMarker::new() {
            let _ = sender.send(work(marker));
        }
    })
    .map_err(|_| "Still is shutting down.".to_string())?;
    // If the event loop is torn down before the closure runs, the sender is
    // dropped and this returns an error instead of hanging.
    receiver.recv().map_err(|_| "Still is shutting down.".to_string())
}
