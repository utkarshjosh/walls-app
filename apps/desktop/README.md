# Still for macOS

The native shell around the shared React UI in `apps/web`: a Tauri 2 app whose
Rust core lists displays, sets the desktop picture (including one picture
spread across several displays), plays video and scene wallpapers in
desktop-level windows, and rotates wallpapers on a schedule even while the
window is closed.

The UI and this core talk through a fixed contract:
`apps/web/src/platform/types.ts`. Change that file and `src-tauri/src/model.rs`
together.

## Run it

```sh
cd apps/desktop
npm install
npm run dev      # starts the UI dev server (apps/web) and the app
npm run build    # builds apps/web, then Still.app and a .dmg
```

Needs Rust (stable), Node 22 and the Xcode Command Line Tools. Build output
lands in `src-tauri/target/`.

Other things you can run from `src-tauri/`:

```sh
cargo test                               # the pure logic: rotation, schedule, span geometry, names
cargo clippy --all-targets
cargo run -- --print-displays            # list displays as the UI receives them, then exit
STILL_DRY_RUN=1 npm run dev              # (from apps/desktop) see below
```

**Dry run.** With `STILL_DRY_RUN=1` the app does everything except touch the
desktop: instead of setting a picture or opening a live wallpaper window it
prints what it would have done. Rotation, the timer, the library and the tray
all work normally, so it is the safe way to poke at the UI.

If the interface's dev server is already running on port 5173, reuse it instead
of starting a second one:

```sh
npx tauri dev --config '{"build":{"beforeDevCommand":""}}'
```

## A release copy

```sh
cd apps/desktop
npx tauri build --bundles app
open src-tauri/target/release/bundle/macos/Still.app
```

A release copy talks to the deployed catalog at `https://walls.utkarshjoshi.com`; a development run talks to `http://localhost:8787`. Both use the same library folder and library list, so quit one before opening the other. The app runs from any folder: `~/Applications` works as well as `/Applications`, which matters on a Mac that does not let you change the latter.

## Releasing a beta

The app is given away from the landing page (decided 2026-11-12). To put a build there:

```sh
cd apps/desktop
npm run build        # Still.app and src-tauri/target/release/bundle/dmg/Still_<version>_aarch64.dmg
```

Then put it on the website, with the version from `src-tauri/tauri.conf.json`:

```sh
../api/deploy/release.sh 0.1.0 mac-arm64 src-tauri/target/release/bundle/dmg/Still_0.1.0_aarch64.dmg "What is new, in a sentence."
```

The same can be done in a browser, in the catalog service's admin portal under **Releases** ([apps/api, "Downloads"](../api/README.md#downloads)). Building the app and deploying the server do not publish anything by themselves. The version number in `tauri.conf.json` is the one place to change it; the interface reads it from there too.

**Which Macs it runs on.** The Rust that Homebrew installs here builds for Apple silicon only, so `npm run build` makes an Apple-silicon build. For Intel Macs there are two ways, neither tried yet:

- push a tag such as `v0.1.0`: `.github/workflows/release.yml` builds one disk image for both kinds of Mac on GitHub and keeps it as a draft release to download; or
- install Rust with `rustup`, run `rustup target add x86_64-apple-darwin`, then `npx tauri build --target universal-apple-darwin`.

**What people see the first time they open it.** The build carries an ad-hoc signature (`"signingIdentity": "-"` in `tauri.conf.json`), which makes the bundle whole but says nothing about who made it. This is how macOS treats any app from a developer it cannot identify: it refuses to open a downloaded copy until its owner approves it once in System Settings, Privacy & Security, "Open Anyway", and a Mac managed by an employer may not allow that at all. The landing page tells people to expect this. Nobody has yet downloaded a copy on another Mac and gone through it, so do that before announcing anything. Without that setting the bundle's signature is incomplete (`codesign --verify` fails on it), which macOS on Apple silicon reports as a damaged app with no way to approve it. The only way to remove the prompt is an Apple Developer Program membership (US$99 a year), a "Developer ID Application" certificate, and notarisation: set `APPLE_SIGNING_IDENTITY` and the `APPLE_ID` / `APPLE_PASSWORD` / `APPLE_TEAM_ID` variables and `tauri build` does both. Not done.

**Signing in.** The More menu offers "Sign in…" when the catalog service says it can send codes (`GET /v1/auth/status`); against a service that cannot, the entry is not there. While someone is signed in, `apps/web/src/api/account.ts` reports what they do with catalog wallpapers; signed out, it reports nothing. The token is kept in the window's local storage.

## What has actually been run (2026-11-10)

On macOS 27 with two displays attached (a built-in Retina display on the left
and an external 1920x1080 display), unsigned development build, not sandboxed.
Screen capture was not available, so results were read back from macOS and from
the live windows themselves rather than looked at.

| Checked | Result |
| --- | --- |
| Same picture on every display | macOS reported the library file as the desktop picture of both displays |
| One picture across both displays | Each display got its own piece at its own pixel size (3456x2234 and 1920x1080); joined side by side they form one continuous picture |
| Different picture on each display | One per display, left to right |
| One display only | Only that display changed; the other kept its picture |
| Live scene | One window per display at the desktop level, exactly covering it, in front of the system wallpaper and behind the Finder's desktop icons; WebGL running in both; spanned geometry correct |
| Live video | Playing and looping in both windows through the asset protocol, about one millisecond apart |
| Back to a still picture | The live windows closed |
| Next / Previous / Pause | Desktop pictures followed; the next change was scheduled on the local clock hour |
| Quit and relaunch | State restored; the live window came back on the right display; nothing else changed |
| Real interface in the window | Copied its nine starter wallpapers to disk, synced a paused plan, did not touch the desktop, reported no page errors |
| Release copy | Opened with the library the development copy had been using (8 collections, 28 wallpapers), kept the wallpaper, and its requests arrived at the deployed catalog |
| Resolution rule, "same on each" | With a 1672x941, a 3840x2400 and a 7680x2400 picture in rotation, the small one never appeared (it is too small for the Retina display) |
| Resolution rule, "one across all" | Only the 7680-wide picture was stretched into two pieces; the 3840-wide one was shown on each display for its turn; the small one never appeared |
| Resolution rule, "different on each" | The Retina display only received the two large pictures; the 1080p monitor also received the small one; frames alternated and did not freeze |
| Stretching asked for by hand | The small picture was shown on each display instead of being stretched; the panorama was stretched |
| Pictures with no recorded size | The app measured the files itself and applied the same rule |

From the window itself, the owner started rotation, chose "one across all" and
pressed Next several times during the same session; the saved state and the
desktop pictures matched. Not exercised inside the window: adding files, saving
from Discover, and setting a live wallpaper. Not exercised at all: the menu-bar
item, closing and reopening the window, unplugging a display while running,
battery pause, a scheduled change firing after hours in the background, other
Spaces, and anything inside the App Store sandbox.

## Where things live at run time

| What | Where |
| --- | --- |
| Library files | `~/Library/Application Support/com.utkarshjoshi.still/library/` |
| The library list (collections, wallpapers, settings) | `library-state.json` in that same folder. Written by the interface; a development run and a release build share it |
| Rotation state (plan, history, next change) | `~/Library/Application Support/com.utkarshjoshi.still/rotation.json` |
| Pieces of a picture spread across displays | `~/Library/Caches/com.utkarshjoshi.still/span/` |

Deleting `rotation.json` resets rotation; the UI sends the plan again on its next sync.

## Layout

```
src-tauri/
  tauri.conf.json            window, CSP, asset-protocol scope, bundle
  tauri.appstore.conf.json   extra settings for a Mac App Store build
  capabilities/main.json     what the main window may call (live windows: nothing)
  build.rs                   declares the commands so they are denied by default
  Entitlements.plist         sandbox entitlements (App Store build only)
  Info.plist                 merged into the bundle's Info.plist
  src/
    model.rs      the contract's types
    engine.rs     which wallpaper comes next            (pure, tested)
    schedule.rs   when the next change is due           (pure, tested)
    fit.rs        is a wallpaper large enough for a display (pure, tested)
    library.rs    the folder of wallpaper files
    scene.rs      is an arriving scene file safe to keep (pure, tested)
    displays.rs   attached displays, via AppKit
    wallpaper.rs  span geometry (pure, tested) and setting desktop pictures
    live.rs       desktop-level windows for video and scene wallpapers
    power.rs      battery or mains, via IOKit
    rotation.rs   the worker thread that owns state, timer and persistence
    tray.rs       the menu-bar item
    commands.rs   what the UI can call
    lib.rs        app lifecycle
scripts/make-icons.swift     draws the placeholder icons
scripts/parity-traces.mjs    records the TypeScript rules' answers (see below)
```

How the threads fit together: AppKit is only touched on the main thread. One
background thread, the rotation worker, owns all rotation state and is the only
thing that changes wallpapers; commands, the tray and display notifications
post messages to it. The worker hops to the main thread for AppKit calls, and
the main thread never waits for the worker, so the two cannot deadlock.

## Two copies of the rules

Which wallpaper comes next (`engine.rs`) and whether a wallpaper is large
enough for a display (`fit.rs`) are ports of `apps/web/src/lib/rotation.ts` and
`fit.ts`, which drive the UI and the browser preview. The unit tests mirror the
TypeScript tests case for case. After changing either copy, check that they
still agree on a few thousand generated cases:

```sh
cd apps/desktop
node --experimental-strip-types scripts/parity-traces.mjs ../web /tmp/still-traces.json
cd src-tauri && STILL_PARITY_TRACES=/tmp/still-traces.json cargo test parity -- --nocapture
```

The script only reads `apps/web`. Without `STILL_PARITY_TRACES` the Rust test
passes without checking anything, so plain `cargo test` needs no Node.

How sizes are used: automatic changes (timer, Next, the first wallpaper after
Resume) only draw from wallpapers large enough for where they would go, and a
wallpaper is stretched across displays only when it has the pixels for it;
otherwise it is shown on each display for its turn. If nothing is large enough,
nothing changes. A wallpaper chosen by hand is shown regardless. A picture that
arrives without a size has it read from the file.

Changing the display mode or connecting a display never puts a wallpaper on a
display it is too small for. Still keeps a showing wallpaper that can go
everywhere; failing that it makes one automatic change; failing that it leaves
every display as it is (the status then lists displays one by one, even in
"same" or "span" mode). A wallpaper already on a display stays there.

A scene marked `perDisplay` (a clock) draws itself whole in every window, so it
is never one picture across displays: in span mode, or when spanning is asked
for by hand, it is laid out and recorded as "same", poster included.

## Scenes are checked when they arrive

A scene is an `.html` file, which means running code, so it is contained in
layers (`docs/16-live-scene-standard.md`). The app is layer 3. When an `.html`
file is stored or downloaded, `scene.rs` keeps it only if it carries the
standard's content policy: exactly one
`<meta http-equiv="Content-Security-Policy">`, with exactly the standard's
`content`, ahead of any script and with nothing in front of it but the doctype,
the `<html>` and `<head>` tags and `<meta charset="utf-8">`. That last part is
what makes the policy certain to be applied: the same line inside a comment or
in the body would be in the file and ignored by the web engine. A file that
fails is never written into the library and the person sees "That live
wallpaper is not in a form Still can run safely." Scenes already in the library
are not re-checked, so ones saved under an earlier policy text keep playing.
Only a file named `.html` can be played as a scene.

## Icons

The icons are placeholders drawn by `scripts/make-icons.swift`. To replace them,
edit the script (or put your own 1024x1024 PNG at
`src-tauri/icons/source/app-icon.png` and a black-and-transparent 36x36 PNG at
`src-tauri/icons/tray-template.png`), then:

```sh
npm run icons
```

## Mac App Store

Nothing here signs or submits anything. This section is what a sandboxed
App Store build still needs, and where the risks are.

### What you still need

1. **Full Xcode**, not just the Command Line Tools. The upload tool
   (`xcrun altool`) ships only with Xcode; Apple's separate Transporter app is
   the alternative.
2. **An Apple Developer Program membership**, and in App Store Connect an app
   record with the bundle id `com.utkarshjoshi.still`.
3. **Two signing certificates** in your keychain: "Apple Distribution" (signs
   the app) and "Mac Installer Distribution" (signs the `.pkg`).
4. **A Mac App Store provisioning profile** for that bundle id, saved as
   `src-tauri/embedded.provisionprofile` (git-ignored).
   `tauri.appstore.conf.json` copies it into the bundle.
5. **Your Team ID in `Entitlements.plist`**: uncomment the
   `application-identifier` and `team-identifier` entries and replace `TEAMID`.
6. **A build for both chip types, or a newer minimum macOS.** The Homebrew Rust
   on this machine only builds for Apple Silicon. Either install Rust with
   rustup and add `x86_64-apple-darwin` to build
   `--target universal-apple-darwin`, or ship Apple-Silicon-only. As far as I
   know the App Store accepts an Apple-Silicon-only app only when its minimum
   system version is 12.0 or later, which is what `tauri.conf.json` sets
   (the display code also needs at least macOS 10.15).
7. Then, roughly (Tauri's "App Store" distribution guide has the details):

   ```sh
   cd apps/desktop
   npx tauri build --bundles app --config src-tauri/tauri.appstore.conf.json
   # with APPLE_SIGNING_IDENTITY="Apple Distribution: <name> (<TEAMID>)" in the environment
   xcrun productbuild --sign "3rd Party Mac Developer Installer: <name> (<TEAMID>)" \
     --component src-tauri/target/release/bundle/macos/Still.app /Applications Still.pkg
   xcrun altool --upload-app --type macos --file Still.pkg --apiKey <key id> --apiIssuer <issuer id>
   ```

   I have not run these steps; treat them as a starting point.

### Things that may behave differently, or be questioned, under the sandbox

None of this has been run inside the sandbox. "Believed fine" means a public
API that other App Store apps are known to use; "unsure" means exactly that.

- **Setting the desktop picture** (`NSWorkspace.setDesktopImageURL`). Public,
  and used by sandboxed wallpaper apps. The picture file must be readable by
  the system's wallpaper process, not only by Still. Files inside the app's
  own container have worked for other apps; **unsure** whether that still
  holds on current macOS without testing.
- **Spanned pieces live in the Caches folder** (as specified). macOS may empty
  an app's Caches when disk space is low. The desktop would then lose its
  picture at the next login until Still re-applies. If that proves a problem,
  move the `span` folder to Application Support (one line in `lib.rs`, plus
  the asset-protocol scope in `tauri.conf.json`).
- **Desktop-level live windows** (`NSWindow.level` from
  `CGWindowLevelForKey(kCGDesktopWindowLevelKey)`, mouse events ignored, on all
  Spaces). All public AppKit. Believed fine; live-wallpaper apps on the Store
  do the same.
- **Transparent web views.** The live windows are transparent, and the main
  window has a background colour. For both, Tauri's web view layer (wry) sets
  `drawsBackground` on WKWebView through key-value coding. That key is not
  documented by Apple. Tauri enables it unconditionally since 2.12.1 and it is
  common in shipping apps, but it is the one place this app leans on
  undocumented behaviour. **Unsure** how review treats it.
- **Power source** (`IOPSCopyPowerSourcesInfo`,
  `IOPSGetProvidingPowerSourceType` from IOKit). Public, read-only, no
  entitlement known to be required. Believed fine.
- **Downloads** use `ureq` with rustls and bundled root certificates, covered
  by the `network.client` entitlement. It does not use the system proxy
  settings or the system keychain's extra trusted roots, so it can fail on
  networks that require either.
- **Importing files.** The Import dialog uses `<input type="file">`. A
  sandboxed app can only show the system file picker with
  `com.apple.security.files.user-selected.read-only`. It is in
  `Entitlements.plist`, commented out, because the brief asked for sandbox and
  network only. Expect to need it. Dragging files onto the window is
  **unsure**; it normally works without the entitlement.
- **Single instance.** `tauri-plugin-single-instance` talks over a socket in
  `/tmp`, which the sandbox blocks; the plugin then quietly does nothing. That
  is fine for normal launches (macOS itself keeps one copy of a bundled app
  running) but means `open -n` could start a second timer in a Store build.
- **File locations.** The app-data and cache folders are expected to move
  inside the sandbox container automatically. **Unsure** until run sandboxed;
  an existing unsandboxed library is not migrated.
- **Not included:** launch at login. Rotation runs while the app is running
  (window closed or not); after a restart it resumes when Still is next opened.
  A Store build would use `SMAppService` for this.
- **Timer accuracy while hidden.** macOS slows the timers of apps with no
  visible window ("App Nap"). Still wakes at least every 30 seconds and at each
  boundary, but how late a change can arrive after hours in the background is
  untested. If it matters, opt out with `NSProcessInfo.beginActivity`.
- **One macOS limitation to know about:** historically `setDesktopImageURL`
  changes the picture only for the Space currently showing on each display;
  other Spaces keep theirs unless "Show on all Spaces" is on in System
  Settings > Wallpaper. Not checked on this macOS version. Still does not
  re-apply when you switch Space.
