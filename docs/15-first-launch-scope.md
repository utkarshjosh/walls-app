# First launch: what ships, and what must not be built yet

Written 2026-11-10 alongside the first working build. Target then: a paid Mac app on the Mac App Store. This page supersedes earlier documents wherever they disagree; the changed decisions are listed at the end.

## Changed on 2026-11-12

The owner changed the direction two days after this page was written. Read the rest of the page with these in mind; where they disagree, this section wins.

| This page says | Now |
| --- | --- |
| A paid app on the Mac App Store | Free, in beta (versions stay below 1.0), downloaded straight from the landing page. Not being sold for now. |
| Item 1: no sign-in of any kind | Optional sign-in by a six-digit code sent to an email address. No passwords, no outside auth service: the catalog server does it and rents a mail relay to send the code. The app works the same signed out. |
| Item 26: no analytics; "no data collected" | For people who sign in, the server records which catalog wallpapers they open, save, set and remove, and what they search for, to drive discovery and suggestions later. Nothing is recorded about anyone signed out beyond the plain daily totals. The landing page has a privacy page that says exactly this. |
| Items 10 and 11: no direct-download build, no updater of our own | Direct download is the only channel. Builds are uploaded in the admin portal and offered by the landing page. There is still no updater: a new version is a new download. |
| Item 4: the admin portal is read-mostly | It now also manages users (see what they did, block, remove) and releases (upload builds, publish, take back). It still does not edit the catalog. |
| Item 12: no other platforms | Unchanged: the app runs on macOS only. The download pages and the server accept Windows and Linux builds, so adding one later needs no change there, but no such build exists. |
| The landing page collects addresses for an early build | The form is gone. The list it made is kept in the portal ("Early list"). |

What this adds to "What does have to happen before launch": a mail relay account and a verified sending domain (sign-in is off until then); an Apple Developer ID and notarisation, or accepting that every Mac asks its owner to approve the app by hand the first time; an Intel or universal build if Intel Macs are to be supported; a contact address on the privacy page.

## What the first version is

One Mac app with three sections and nothing else.

| Section | What it does in v1 |
| --- | --- |
| Library | Your wallpapers, kept as files on this Mac, grouped into collections. Add pictures or videos, view full size, set as wallpaper. |
| Discover | Wallpapers and ready-made collections served by our small catalog server. Save one, or save a whole collection. |
| Rotation | Which collections take turns on the desktop, how often, shuffled or in order, with Previous / Pause / Next. Keeps running with the window closed, from the menu bar. |

Three things make it worth paying for:

- **Multiple displays, with no setup.** With one display the app shows no display options at all. Connect a second and three choices appear: same on each, one wallpaper across all, different on each. "Across all" follows the real arrangement of the displays.
- **Never a blurry desktop.** Every wallpaper is checked against the real pixel size of the displays connected. Rotation leaves out pictures that are too small, and only stretches a picture across displays when it has the pixels for the densest one.
- **Live wallpapers.** Animated scenes from Discover (tiny files, any resolution, and they span displays seamlessly) and your own video files.

The server's only job is the Discover catalog: a single Go program, a SQLite file, and the object storage bucket.

## Do not build these before launch

Each of these was either in the earlier plan or is a tempting next step. None is needed to charge for the app, and each one adds weeks, an ongoing cost, or an App Store review risk.

### Accounts and anything that needs them

1. **Sign-in of any kind (Google, Apple, email).** The App Store already knows who paid. No login means no auth provider, no password resets, no account deletion flow, and no obligation to add Sign in with Apple.
2. **Syncing a person's library between Macs, or uploading their files to our server.** The library is local files. This is also what keeps the bucket inside its 8 GB limit: it holds our catalog only, never customer uploads.
3. **PostgreSQL, a job queue, a worker process, device pairing, seven-day plans.** All of that existed to support accounts and sync. The catalog needs none of it.
4. **An admin website that changes the catalog.** Content goes in with one command (`walls ingest <folder>`). Changed by the owner on 2026-11-10: a small read-mostly admin portal now exists at `/admin` for the early-build list and daily totals (see "Decisions this changes"). It lists the catalog but does not edit it.

### Community and "smart" features

5. **Wallpapers submitted by users, profiles, likes, comments, follower counts.** Public user content needs moderation, a reporting and blocking flow (an App Store requirement), and a rights process. Discover in v1 is curated by us only. Confirmed by the owner on 2026-11-10: there is no community or user base yet, so we publish the wallpapers and collections ourselves and keep the quality up.
6. **Anything AI: taste learning, "describe a wallpaper", generated images on demand.**
7. **Pulling from Unsplash, Pexels, Wallhaven or any other site.** Each has API keys, attribution rules and hot-linking terms. Curate and license a catalog instead.
8. **Engagement mechanics: streaks, "wallpaper of the day" notifications, badges, sharing to social networks.** Sharing wallpapers socially is ruled out for now by the owner, not merely postponed.

### Payments and distribution

9. **Our own payments, licence keys, subscriptions, trials or a paywall.** A paid-up-front App Store listing needs no payment code at all. Decide the price, not a billing system.
10. **Our own updater.** The App Store delivers updates.
11. **A direct-download build with its own signing and notarisation pipeline.** One distribution channel until it works.

### Other platforms

12. **Windows, Linux, Android or iOS versions.** The interface is React and the native part sits behind one small contract, so a second platform stays possible. Do not start it.
13. **A website version of the app.** The browser build exists only as a design preview.

### Depth in display and wallpaper control

14. **Per-display schedules, per-Space (virtual desktop) wallpapers, or different timing per display.** One schedule for everything.
15. **Compensating for the physical gap between monitors, or a manual crop/offset editor for spanning.** Automatic span from the arrangement is the feature.
16. **Image editing: crop, blur, dim, tint, filters.**
17. **Time-of-day, light/dark, sunrise or weather-driven wallpapers.**
18. **Noticing wallpaper changes made in System Settings and reacting to them.** The next scheduled change simply replaces it.
19. **Lock screen or screen saver integration, Shortcuts actions, widgets, a menu-bar popover with thumbnails.** The menu bar item is a plain menu.

### Depth in live wallpapers

20. **Hosting videos in the catalog.** A good loop is 50–200 MB; a few dozen would fill the bucket. Catalog live wallpapers are scenes (a few kilobytes each). People can still add their own videos locally.
21. **Scenes submitted by users, a scene editor, interactive or audio-reactive wallpapers, scenes that fetch live data, web pages as wallpapers.** Scenes run code. We may adapt other people's work when its licence allows, but every scene is built with our kit, passes the gate and is reviewed by us before it is published ([live scene standard](16-live-scene-standard.md)).
22. **Video conversion or trimming.** Accept MP4, MOV and WebM as they are.

### Polish that can wait

23. **A light theme, user-selectable themes, or an in-app accent picker.** One dark theme, one accent.
24. **Languages other than English.**
25. **An onboarding tour.** The first screen already shows starter collections and a Start button.
26. **Analytics or third-party crash-reporting SDKs.** "No data collected" is the simplest App Store privacy label, and a selling point. The server does keep plain daily totals of requests it already receives (Discover opened, a wallpaper's file fetched), with no IP address or identifier; the app sends nothing extra. Before submitting, confirm the owner is comfortable that this still fits the "no data collected" label.
27. **A CDN, multiple regions, or redundancy.** One small server and the bucket. Add a cache in front only if traffic demands it.

## What does have to happen before launch

Short, and mostly not code.

| Item | Why it blocks launch |
| --- | --- |
| Final name and app icon | "Still" is a working name and the icon is a placeholder. The bundle identifier cannot change after release. |
| A real catalog | The seed is our own generated artwork plus five scenes. Aim for a few hundred licensed or original wallpapers so Discover feels alive. |
| Apple Developer Program and full Xcode | Needed to sign a sandboxed build and submit it. US$99 per year. |
| Sandbox check on a signed build | Confirm that setting wallpapers per display, the behind-the-icons live windows, and file access all work inside the App Store sandbox. This has not been tested. |
| Open at login | People expect a wallpaper changer to survive a restart. Small, but must exist. |
| Wallpaper on other Spaces | macOS applies a desktop picture to the Space that is showing. People who use several Spaces will see the old picture on the others until the app re-applies when the Space changes. Not built. |
| Permission to open the file picker | Under the App Store sandbox, "Choose files" needs the user-selected-files entitlement. It is prepared but switched off. |
| Full click-through of the Mac app | The native commands were tested on a real two-display setup and the interface in the browser. Inside the app window only starting rotation, spanning and Next have been used; adding files, saving from Discover and live wallpapers still need a pass there. |
| ~~Deploy the catalog server~~ | Done 2026-11-10: https://walls.utkarshjoshi.com on `agent-server-oci`, behind Cloudflare, serving from the server's disk. Still open: a Cloudflare cache rule for `/v1/files/`, and deciding whether the bucket is needed at all. |
| Energy check for live wallpapers | Measure battery cost on a laptop before promising it. |
| Privacy policy and support pages | Required by the App Store listing. The landing page exists (`apps/site`) but has neither page yet: both need a contact address and the owner's review of the wording. |
| Choose the accent colour | Champagne is the working default; sage and clay are one line away. |

## Decisions this changes

| Earlier decision | Now |
| --- | --- |
| ADR-016: direct download, no Mac App Store | Mac App Store is the launch channel. |
| ADR-018 and roadmap stages 1–2: Google sign-in, pairing, server-compiled plans | Deferred until after launch; v1 has no accounts. |
| ADR-002: PostgreSQL | Deferred with accounts. The catalog uses SQLite. |
| ADR-007: same image on every display | Replaced by three display modes that appear only with two or more displays. |
| ADR-013: discovery deferred | A curated, read-only Discover ships in v1; user submissions stay deferred. |
| Live wallpapers were not planned | In v1 as first-party scenes plus the user's own videos. |
| No admin website; the server is read-only | 2026-11-10: the landing page collects email addresses for an early build, and the owner has a password-protected portal at `/admin` showing that list, daily totals and the catalog. Still no user accounts: one owner password, nothing for customers to sign in to. The privacy page (already on the launch list) now also has to cover the sign-up list. |
