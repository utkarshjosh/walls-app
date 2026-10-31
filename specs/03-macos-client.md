# macOS companion specification

Status: proposed v0.2. This is a native user-session application, not a kernel extension or browser plugin. Covers FR-10–15 and the macOS portion of FR-24.

## Native integration

SwiftUI menu bar UI with a small AppKit bridge. Use `NSWorkspace.setDesktopImageURL(_:for:options:)` on the main thread for each current `NSScreen`; downloads and checksum work stay off the UI thread. Use local file URLs and set an explicit fit policy. Default `fill` may crop; reserve fit adjustments for secondary image settings if needed, not onboarding. [Apple API](https://developer.apple.com/documentation/appkit/nsworkspace/setdesktopimageurl(_:for:options:))

Start-at-login uses the public `SMAppService` lifecycle and system approval/status UX. No root daemon, private wallpaper database mutation, or Accessibility permission should be required for the chosen public API path; verify that assumption during the spike. [Apple service registration](https://developer.apple.com/documentation/servicemanagement/smappservice/register())

Proposed minimum macOS 14; validate against the owner's actual OS and target release before committing support. Public AppKit works with current screens, but that alone does not prove independent behavior for every Mission Control Space, full-screen desktop, or lock screen. Record the real behavior in the spike and describe supported scope accurately.

## Components and local state

`APIClient` owns HTTPS/device auth; `SyncCoordinator` owns the state/manifest/grant sequence; `CacheStore` owns staging, checksums, safe paths, and eviction; `PlanStore` persists active and pending manifests; `RotationController` evaluates due entries; `DesktopAdapter` applies native changes; `MenuModel` exposes state.

Use URLSession, Keychain for the high-entropy device credential, and SQLite or atomic local metadata files for state. Keep artifacts in an app-owned Application Support directory with user-only permissions. Persist active manifest, last applied entry, skipped-entry override, pause state, sync date, blocked variants, and pending event batches. Never execute shell commands or use server-supplied paths; local filenames are derived from validated hashes.

## Pairing and authentication

Onboarding already selected starter/private folders and generated default rotation preferences. After browser approval, fetch the first plan and begin automatically. Capture the system timezone and report it on subsequent syncs; do not ask the user to configure clocks or a cache.

Client asks server for a 10-minute pairing challenge, opens the HTTPS verification URL, and displays the short user code. Server approval occurs only in the signed-in browser; code expiry, rate limits, device name, and one-time consumption are mandatory. Poll no faster than five seconds, with server slowdown guidance. A custom protocol URL is optional and must not carry secrets.

Successful pairing yields a random 256-bit opaque device token, stored only in Keychain. Server stores a keyed hash and scopes it to this device. Proposed lifetime is 90 days; rotate through an authenticated endpoint during daily sync after 30 days, with a bounded 5-minute previous-token grace for an in-flight request. Reuse after grace is rejected. An expired token requires pairing again; cached playback is independent until the client learns access was revoked/expired.

## State machine

`unpaired → pairing → idle → syncing → staging → active`, with `offline`, `paused`, `degraded`, and `reauth_required` overlays. Do not confuse downloaded with activated. Primary UI shows Connected, Paused, or Needs attention. Last activation, cache coverage, and errors are available in secondary diagnostics; transient offline/downloading conditions recover quietly.

- **Pause:** stop applying automatic changes until resumed. Downloads may continue. Persist across launches.
- **Next:** choose the next eligible manifest entry's image immediately, holding it until the next scheduled boundary; does not shift future UTC timestamps. Repeated Next moves forward in entries and wraps within the valid plan. Disable if there is no other eligible image.
- **Open collection:** open the web collection view; primary control alongside Next and Pause/Resume.
- **Sync now (secondary):** run state fetch and staging immediately; manual pause still takes precedence over applying a newly activated plan.
- **Resume:** clear pause/skip and apply the currently due eligible entry.

Respect manual wallpaper changes: before an automatic change compare the current public desktop-image URL with the last URL the app applied. If a different image was chosen externally, pause that session and show a Resume action. Do not fight another wallpaper app. Validate URL-normalization behavior in the spike; where detection is unreliable, expose the limitation instead of touching private OS storage.

## Wake, display, and failure behavior

Observe wake and screen-configuration notifications. Re-evaluate current time once on wake; apply only the current item and catch up a missed sync once. New displays receive the current item after a short debounce. MVP uses the same image on all attached displays. Record per-screen application results; a disconnected display is not an application crash.

A failed native API call preserves the previous screen image and retries quietly; show a concise action only for persistent failures, with technical details in diagnostics. Keep the underlying active file while macOS references it. Verify paths stay inside the cache and resist symlink traversal. Prevent multiple app instances from competing through a process-local/session lock.

## Distribution and removal

Development: local signed development build and explicit manual launch. Shared beta: Developer ID signing, hardened runtime, notarization, versioned installer/DMG, and documented update verification. Investigate signing membership/cost before distribution; do not tell users to globally disable Gatekeeper. Automatic updates are deferred until a signed update channel is validated.

On disconnect, revoke the credential when online, remove it locally, unregister login launch, cancel downloads, and optionally erase cache. Save the pre-app desktop URL/options at first use and restore them if still accessible, otherwise let the user choose an image. Removing the app must not leave a privileged service running.

## Mobile boundary

Android is the likely first automatic mobile client; Android exposes `WallpaperManager`, but background work is OS-managed and should not promise exact timers. [Android API](https://developer.android.com/reference/android/app/WallpaperManager)

iOS is a separate feasibility track: start with downloading/exporting portrait images and a user-configured Shortcuts path if verified on the target version. This design makes no promise that a third-party iOS app can silently set system wallpaper on a server-defined schedule. Capability negotiation prevents desktop assumptions leaking into mobile clients.
