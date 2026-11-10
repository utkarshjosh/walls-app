# Prototype Instructions

Run the local server yourself and open the preview in the browser available to this environment. Do not give the user server-start instructions when you can run it.

Before making substantial visual changes, use the Product Design plugin's `get-context` skill when the visual source is unclear or no longer matches the current goal. When the user gives durable prototype-specific design feedback, preferences, or decisions, record them in `AGENTS.md`.

When implementing from an explicitly selected generated mock, preserve its agreed layout and hierarchy. The current component-first direction below supersedes a default mock-generation workflow; use the selected component source and shared tokens, and validate the working UI with real assets.

Build app UI in `src/`. Keep `.openai/hosting.json`, `worker/index.js`, `scripts/prepare-sites-build.mjs`, and `tests/sites-worker.test.mjs` intact so the same local prototype can be handed to Sites. Before a Sites handoff, run `npm run build` and `npm run test:sites`; the build must leave `dist/client/index.html`, `dist/server/index.js`, and `dist/.openai/hosting.json`.

## Owner direction — 2026-11-10 (latest; wins over anything older below)

The owner's stated priorities, in order: (1) interface quality good enough that people pay for the app; (2) multiple displays, including one wallpaper spanned across them, with an extremely simple experience where **features appear only when the hardware is there** — with one display the app shows no display options at all; (3) live wallpapers; (4) a Discover section. Launch target is the **Mac App Store**. React for the interface (to stay cross-platform), Go for the server.

What is deliberately not being built before launch is listed in [docs/15-first-launch-scope.md](../../docs/15-first-launch-scope.md). Do not add accounts, sync, an admin site, AI, or user-submitted content without the owner changing that list.

Implemented in this folder as of 2026-11-10 (TypeScript, Tailwind 4, Radix, Motion):

- `src/platform/` is the only door to the host. `types.ts` is the contract; `web.ts` is the browser preview (files in IndexedDB, rotation on a page timer, simulated displays); `desktop.ts` calls the Tauri commands in `apps/desktop`. Screens never import Tauri.
- `src/state/store.tsx` owns the library (local-first; older prototype data is migrated by `restore()` in `model.ts`). `src/lib/schedule.ts` and `src/lib/rotation.ts` are the timing and ordering rules, mirrored in Rust and covered by `npm test`.
- `src/pages/` has the three sections. Library = collections → wallpapers → full-size viewer, import, right-click and button menus. Discover = catalog from `apps/api` with Save / Saved, ready-made collections, a Live filter, and a panoramas filter that exists only with 2+ displays. Rotation = display preview, Previous / Pause / Next, collections, timing, and a Displays section that exists only with 2+ displays.
- `src/components/ui/NOTICE.md` records which Shadcn Space components were adapted and how.
- The browser preview has a **Preview tools** menu (never shown in the Mac app) to pretend 1–3 displays are attached and to compare the three accent proposals. Champagne is the working default; the accent is still not finally chosen.
- A fresh install starts with rotation **paused** and never touches the desktop until the person presses Start. Keep it that way.
- In the browser, wallpaper actions are labelled as a preview ("In preview", "Showing in the Rotation preview"). Never word them as if the real desktop changed.

### Resolution — owner feedback, 2026-11-10

After using the first build on a 16-inch MacBook Pro beside a 24-inch 1080p monitor, the owner called it "a blunder" that the app put small pictures on a Retina display and stretched them across both displays. The rule now, and it is not optional:

- Every wallpaper keeps its pixel size. Whether it is large enough is always judged against the displays connected right now (`src/lib/fit.ts`, mirrored in the native core).
- Automatic rotation never shows a wallpaper that would look blurry. In "one across all" a picture is stretched only if it has the pixels for the densest display; otherwise it is shown on each display for its turn, or skipped if too small even for that.
- By hand a person may still set a small picture on a display, but the interface says "Too small" first, and stretching a too-small picture across displays is not offered.
- Starter wallpapers must be sharp on a Retina display. The original nine 1672 x 941 samples are no longer given to new installs.

Also from the same session: with the full-size viewer or a dialog open, the top strip must stay grabbable so the window can be moved (`data-tauri-drag-region`). Any new full-window overlay needs the same.

### Rotation, live scenes and sharing — owner feedback, later on 2026-11-10

- **Displays must be identifiable.** Wherever the arrangement is drawn, each display carries its name and a laptop or monitor icon.
- **The display stage has one size.** The owner switched macOS to a top-and-bottom arrangement and the preview grew until the page had to scroll: "UI consistency is poor here". The stage is a fixed-height box and the arrangement is scaled to fit inside it in both directions (`DisplayPreview` with `contain`). Never let a drawing size itself from its content.
- **In "different on each", the person chooses.** Each display in the stage has a Choose control that opens a picker for that display. Rotation still takes over at the next change.
- **Rotation must scale and stay tight.** Collections are a grid of on/off cards, not a list that pushes everything else down; transport and timing share one row; explanatory sentences are kept to a few words. The owner's words: "less slop and scalability in rotation page".
- **More motion and feedback, still calm.** Asked for: "motion graphics, feedback". In use now: wallpapers cross-fade in the stage, the selected display mode slides, controls dip when pressed, check marks pop in, tile pictures ease in under the pointer. Still not allowed: animating the gallery's layout, springs that overshoot, anything that loops by itself. No blur behind controls on tiles: it makes long grids stutter in the Mac app's web view.
- **Live scenes follow a written standard** ([docs/16-live-scene-standard.md](../../docs/16-live-scene-standard.md)) so they can be made from web material with the kit instead of hand-coding, and every scene passes the same security gate because it is running code.
- **No sharing, no community yet.** Owner decision: "we are prohibiting social sharing of these wallpapers, we don't have a community or userbase yet, we will deploy wallpapers and maintain quality, collections". Discover is published and curated by us only. Do not add share, export-to-social, submit or follow features.

## Wallpaper product direction

Current direction, 2026-11-08: use the component-first foundation in [docs/14-component-foundation.md](../../docs/14-component-foundation.md) and its [component catalog](../../docs/design/component-catalog.json). The owner approved finding and reusing existing components instead of repeatedly inventing the UI through generated mocks. Use Shadcn Space's Radix variants for the common controls; trial Space UI's gallery with local assets. Planes Photo Lightbox remains optional and paid. Preserve licenses and distinguish shortlisted components from installed or validated ones.

The older light/green collection-shelf image is historical context, not the current mandatory target. The owner rejected cyan/blue and prefers restrained, minimal UI; the current exploration uses a neutral dark shell and a nonblue accent. Champagne, sage and clay have not been finally selected. “Still” is only a working name. Do not promote a generated mock or suggested palette to an approved visual target.

The intended product jobs are Library, Discover and Rotation. Keep all three primary navigation labels visible when implementing that broader surface. Library should show collection imagery and immediate browsing/import actions. Do not add a media-player strip, permanent rotation status, routine sync-success messages, duplicate collection navigation, or decorative subtitles. Reveal details where useful and surface actionable errors. Save/Saved feedback remains necessary in Discover. Rotation owns collection selection, order and interval settings, with explicit Previous wallpaper, Pause and Next controls; Previous follows actual wallpaper history, including shuffled rotation. Application Back navigation is separate.

Start implementation with a small Library/open-image/import/actions slice using the selected source components. Discover, cloud sync and native wallpaper application remain future integration work. Preserve current local persistence, import validation and scheduling behavior. Never present local preferences or a timing preview as working desktop/cloud integration. No timeline, AI, metrics dashboard or admin UI in the consumer surface. Motion stays subtle, respects Reduced Motion and never animates gallery layout. Keep shared tokens and components easy to evolve.

Settings now includes frequency/order/collection controls, a daily-at time, and next-change timing preview. This user-requested daily time supersedes the earlier no-time-picker default. Keep one next-change summary, not a future queue. Distinguish local timing preview from actual native wallpaper application.

Hourly preview changes happen on whole local hours; three-hour previews use 00:00/03:00/06:00/etc. Do not offset them by the time the app was opened or configured. Daily changes use the user-selected time.

Collection management belongs on the main Collections screen. Settings/Edit dialogs show only a fixed-size selected-count summary and Manage collections link; never render an expanding collection checklist inside a modal. This supersedes the earlier inline collection controls in Settings.
