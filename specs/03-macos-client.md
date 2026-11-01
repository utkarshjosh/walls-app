# macOS desktop application specification

Status: proposed v0.3. A Tauri 2 user-session app with bundled React collections and a Rust background core, not a kernel extension or browser plugin. Covers FR-10–15, FR-30/32, and the macOS portion of FR-24. See the [desktop/distribution decision](../docs/11-desktop-and-distribution.md).

## Native integration

Tauri hosts the locally bundled React collection window and tray. The Rust core calls a narrow AppKit adapter, using reviewed Objective-C bindings or a tiny Swift bridge chosen during the spike. Use `NSWorkspace.setDesktopImageURL(_:for:options:)` on the main thread for each current `NSScreen`; downloads and checksum work stay off the UI thread. Use local file URLs and set an explicit fit policy. Default `fill` may crop; reserve fit adjustments for secondary image settings if needed, not onboarding. [Apple API](https://developer.apple.com/documentation/appkit/nsworkspace/setdesktopimageurl(_:for:options:))

Start-at-login uses the public `SMAppService` lifecycle and system approval/status UX. No root daemon, private wallpaper database mutation, or Accessibility permission should be required for the chosen public API path; verify that assumption during the spike. [Apple service registration](https://developer.apple.com/documentation/servicemanagement/smappservice/register())

Proposed minimum macOS 14; validate against the owner's actual OS and target release before committing support. Public AppKit works with current screens, but that alone does not prove independent behavior for every Mission Control Space, full-screen desktop, or lock screen. Record the real behavior in the spike and describe supported scope accurately.

## Components and local state

`APIClient` owns HTTPS/device auth; `SyncCoordinator` owns the state/manifest/grant sequence; `CacheStore` owns staging, checksums, safe paths, and eviction; `PlanStore` persists active and pending manifests; `RotationController` evaluates due entries; `DesktopAdapter` applies native changes; `MenuModel` exposes state.

Use a bounded Rust HTTPS client, Keychain for separate user-session and high-entropy device credentials, and SQLite or atomic local metadata files for state. Keep artifacts in an app-owned Application Support directory with user-only permissions. Persist active manifest, last applied entry, skipped-entry override, pause state, sync date, blocked variants, and pending event batches. Never execute shell commands or use server-supplied paths; local filenames are derived from validated hashes.

The webview only presents the interface. Native commands are typed, scoped operations; never expose generic authenticated URL fetching, arbitrary paths, or shell execution. Explicitly restrict custom commands as well as plugin capabilities. Remote pages cannot access IPC. Reject cross-origin credential forwarding and validate asset download destinations, sizes, and hashes.

Closing the collection window keeps the tray/native core running; quitting stops it. Start-at-login is explicit. Prefer releasing the webview while closed if the spike proves safe, then restore view state on reopening. Wallpaper timing must not depend on JavaScript timers.

## Pairing and authentication

Desktop onboarding starts with browser-assisted sign-in returning to the app, followed by starter/private folder selection and Connect this Mac. The interactive user session authorizes collection CRUD and one-time device registration; registration issues the separate sync credential. Fetch the first plan and begin automatically. Complete the handoff/registration payload schemas before building this flow. Capture the system timezone and report it on subsequent syncs; do not ask the user to configure clocks or a cache.

For browser-first setup or recovery, the client can instead ask for a 10-minute pairing challenge, open the HTTPS verification URL, and display the short user code. Server approval occurs only in the signed-in browser; code expiry, rate limits, device name, and one-time consumption are mandatory. Poll no faster than five seconds, with server slowdown guidance. A custom protocol URL is optional and must not carry secrets.

Successful pairing yields a random 256-bit opaque device token, stored only in Keychain. Server stores a keyed hash and scopes it to this device. Proposed lifetime is 90 days; rotate through an authenticated endpoint during daily sync after 30 days, with a bounded 5-minute previous-token grace for an in-flight request. Reuse after grace is rejected. An expired token requires pairing again; cached playback is independent until the client learns access was revoked/expired.

## State machine

`unpaired → pairing → idle → syncing → staging → active`, with `offline`, `paused`, `degraded`, and `reauth_required` overlays. Do not confuse downloaded with activated. Primary UI shows Connected, Paused, or Needs attention. Last activation, cache coverage, and errors are available in secondary diagnostics; transient offline/downloading conditions recover quietly.

- **Pause:** stop applying automatic changes until resumed. Downloads may continue. Persist across launches.
- **Next:** choose the next eligible manifest entry's image immediately, holding it until the next scheduled boundary; does not shift future UTC timestamps. Repeated Next moves forward in entries and wraps within the valid plan. Disable if there is no other eligible image.
- **Open collection:** open/focus the bundled collection window in this app; primary control alongside Next and Pause/Resume.
- **Sync now (secondary):** run state fetch and staging immediately; manual pause still takes precedence over applying a newly activated plan.
- **Resume:** clear pause/skip and apply the currently due eligible entry.

Respect manual wallpaper changes: before an automatic change compare the current public desktop-image URL with the last URL the app applied. If a different image was chosen externally, pause that session and show a Resume action. Do not fight another wallpaper app. Validate URL-normalization behavior in the spike; where detection is unreliable, expose the limitation instead of touching private OS storage.

## Wake, display, and failure behavior

Observe wake and screen-configuration notifications. Re-evaluate current time once on wake; apply only the current item and catch up a missed sync once. New displays receive the current item after a short debounce. MVP uses the same image on all attached displays. Record per-screen application results; a disconnected display is not an application crash.

A failed native API call preserves the previous screen image and retries quietly; show a concise action only for persistent failures, with technical details in diagnostics. Keep the underlying active file while macOS references it. Verify paths stay inside the cache and resist symlink traversal. Prevent multiple app instances from competing through a process-local/session lock.

## Distribution and removal

Development and early open-source alpha assume no paid membership: source builds and labelled ad-hoc-signed packages, with documented per-app Gatekeeper friction. Recommended wider public release: Developer ID signing, hardened runtime, notarization/stapling, versioned DMG, and documented verification. No Mac App Store submission is planned. Do not tell users to globally disable Gatekeeper. Automatic updates wait for a verified signed update channel. See the [distribution plan](../docs/11-desktop-and-distribution.md).

On device disconnect, revoke the device credential when online, remove it locally, unregister login launch, cancel background downloads, and optionally erase cache. Interactive account sign-out separately revokes the user session and clearly offers to disconnect playback too. Full removal clears both credential classes and local state. Save the pre-app desktop URL/options at first use and restore them if still accessible, otherwise let the user choose an image. Removing the app must not leave a privileged service running.

## Mobile boundary

Android is the likely first automatic mobile client; Android exposes `WallpaperManager`, but background work is OS-managed and should not promise exact timers. [Android API](https://developer.android.com/reference/android/app/WallpaperManager)

iOS is a separate feasibility track: start with downloading/exporting portrait images and a user-configured Shortcuts path if verified on the target version. This design makes no promise that a third-party iOS app can silently set system wallpaper on a server-defined schedule. Capability negotiation prevents desktop assumptions leaking into mobile clients.
