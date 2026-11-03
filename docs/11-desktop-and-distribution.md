# Desktop, web reuse, and direct distribution

Decision proposal, 2026-11-01. Supersedes the SwiftUI-only companion choice. Nothing here is an implemented or published release.

## Recommendation

Use **Tauri 2 + React/TypeScript** for the Mac app, **Go + PostgreSQL** for the service, and a separate **React admin web app**. Bundle the collection interface inside the desktop application. Reuse its components, routes, validation, and API types for an optional browser client. Users browse, upload, organize folders, and adjust rotation in one desktop window; the menu bar handles Next and Pause.

Tauri combines a Rust process with the system webview and supports tray applications. Its runtime model fits this reuse goal; that does not establish this application's memory use or native compatibility, which need measurement. [Tauri architecture](https://v2.tauri.app/concept/architecture/)

| Option | Relevant strength | Decision for this product |
| --- | --- | --- |
| Tauri + shared React | One collection UI across desktop/browser; native Rust core; familiar web contribution path | Recommended, conditional on native lifecycle and resource spike |
| SwiftUI + AppKit | Strong Mac conventions and direct platform integration | Good for a Mac-only tool, but would duplicate the rich collection UI |
| Swift shell + WKWebView | Reuses React with a native Mac host | Fallback if Tauri lifecycle integration proves troublesome; requires a custom bridge and packaging work |
| Electron + React | Established shared web UI approach | Viable, but bundles Chromium/Node; prefer measuring the system-webview approach first |
| Browser/PWA alone | Simple deployment and broad availability | Does not provide the required native wallpaper and background lifecycle integration by itself |

The comparison is engineering judgment, not a benchmark. Electron's process/runtime model is documented in its [process model](https://www.electronjs.org/docs/latest/tutorial/process-model). Mobile reuse is possible at the UI/API level; it does not remove OS wallpaper restrictions.

## Reuse boundaries

- `packages/collection-ui`: collection pages, starter selection, uploads, small rotation sheet, accessible shared components. Depend on an injected API/platform interface, never import native plugins directly.
- `packages/api-client`: typed requests and errors. Browser transport uses same-origin cookies; desktop transport invokes a narrow native broker.
- `apps/desktop`: bundled frontend plus Rust core for HTTP, cache, plan persistence, timers, tray, credentials, and native adapters. Use a small AppKit adapter; select Rust Objective-C bindings or a tiny Swift bridge in the spike.
- `apps/web`: optional browser host for the same collection UI, plus authentication/pairing pages needed from the beginning.
- `apps/admin`: browser-only operations interface using separate routes and server-enforced roles. Share design primitives, not privileged navigation with the consumer app.

Do not load a remotely hosted website into a privileged desktop webview. Bundled code owns native capabilities; remote image bytes and API JSON remain data. Allow external links in the system browser, with no native IPC access. Explicitly scope built-in **and custom** commands; do not assume custom handlers become restricted just because a capability file exists. [Tauri capabilities](https://v2.tauri.app/security/capabilities/)

## Authentication and offline behavior

Two credentials have different purposes. A user session can manage that user's collections. A device token can fetch only its own state, manifests, approved variants, and event endpoints. Never broaden the device token into an account/admin token to make the embedded interface work.

Update 2026-11-03: **Google sign-in is selected**, with **Firebase Auth or Supabase Auth** to be chosen later. Use the selected service's documented SDK/session flow rather than implementing a custom OAuth or refresh-token service. Provider selection must prove external-browser return handling, server-side identity verification, and desktop Keychain storage. Keep native reusable credentials out of the webview. The earlier custom handoff endpoints/lifetimes are provisional sketches, not mandatory architecture; see the [API inventory](../specs/05-api.md).

The frontend prototype can use clearly labelled local sample state without connecting either auth platform. Never ship a development auth bypass in production. This auth decision does not move application data or image storage to Firebase/Supabase: the current Go/PostgreSQL/private OCI plan remains. Map provider identities to stable internal user IDs and enforce ownership and admin roles in the Go API.

After sign-in and starter choice, registering this Mac with a clear Connect action issues the separate sync credential. Browser-first setup can still use the existing short-code pairing flow. Ordinary use never requires visiting a separate collection website. User logout revokes the interactive session; offer an explicit choice to disconnect background rotation as well. Account deletion/revocation invalidates the applicable credentials server-side.

Keep playback, downloads, and wake handling in the native core. Closing the collection window keeps the tray app alive; quitting the app stops it. Start at login is a separate, explicit OS-supported preference. Destroy an idle collection webview if feasible, then recreate it on Open collection. Offline MVP supports cached browsing and playback; disable server mutations with a concise explanation rather than building a conflict-prone offline edit queue.

## Releases without the Mac App Store

No App Store submission is planned. Apple Developer membership and App Store distribution are different choices: membership provides the Developer ID route used by apps distributed directly. [Apple Developer ID](https://developer.apple.com/help/account/certificates/create-developer-id-certificates/)

| Stage | Packaging | User experience and limits |
| --- | --- | --- |
| Personal development | Build from source on the owner's Mac; ad-hoc signing as required | No paid membership assumed; document toolchain and OS compatibility |
| Early open-source alpha | Versioned ad-hoc-signed `.app`/DMG from project releases, with checksums and source tag | Cannot promise a frictionless first launch; Gatekeeper approval may be required and managed Macs can prohibit overrides |
| Wider public release | Developer ID signature, hardened runtime, notarization and stapled ticket; direct DMG | Recommended installation path if membership is funded; still distributed outside the Store |

Tauri documents ad-hoc signing and its remaining Privacy & Security approval requirement. Follow Apple's per-app opening guidance only after a user chooses to trust the release. Do not instruct users to disable Gatekeeper globally or strip quarantine recursively. [Tauri signing](https://v2.tauri.app/distribute/sign/macos/), [Apple opening guidance](https://support.apple.com/en-us/102445)

Apple currently lists **US$99 per membership year**, with regional pricing. This is a future distribution decision, not approved spending or a prerequisite for the planning/feasibility work. Open-source status alone does not guarantee a fee waiver. [Apple membership](https://developer.apple.com/programs/enroll/)

## Open-source trust and contribution

Recommend MIT for the project code to keep reuse straightforward; the license remains a deliberate owner choice before public publication. Image permissions and third-party notices remain separate. A visible repository without an explicit license is not yet an open-source release.

Before public alpha, provide documented local builds, pinned toolchains and lockfiles, a source tag matching each release, CI build logs, checksums, dependency/license inventory, a security reporting route, and a short privacy/network-access explanation. Aspire to reproducible builds, but claim reproducibility only after independently comparing builds. Checksums alone do not prove publisher identity or safety.

Use project release hosting for installers so they do not consume `walls-assets`. Publish arm64 first, with a clear support matrix; add a universal build only after Intel testing. A Homebrew tap can follow, but does not replace signing or notarization. Self-hosters can choose their HTTPS service endpoint in advanced setup without modifying app code.

Begin with manual updates and visible release notes. Automatic updates require a verified update signature, secure key custody/rotation, and rollback handling; updater signatures and Apple code signatures solve different problems. Do not silently download and execute an unverified package. Uninstall instructions must cover login registration, Keychain entries, local cache, and restoring the previous wallpaper.

## Feasibility exit

On the owner's Mac, prove bundled UI → scoped native command → cached AppKit wallpaper, close/reopen window, sleep/wake, login launch, external wallpaper change detection, and token separation. Measure the entire process family with the window open and closed. If lifecycle or idle cost fails, evaluate the Swift/WKWebView fallback before expanding features. None of these checks has run yet.
