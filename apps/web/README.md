# Still — the shared interface

The React interface for the wallpaper app. The same code runs in two places:

- inside the Mac app ([apps/desktop](../desktop)), where it changes the real desktop;
- in a browser tab, as a **preview** — everything works, but "set as wallpaper" only updates the in-app preview, and the page says so.

"Still" is a working name.

## Run it

```bash
npm ci
npm run dev
```

Opens on http://localhost:5173 (the Mac app's development build loads this same address). Discover needs the catalog server from [apps/api](../api) on http://localhost:8787; set `VITE_API_URL` to point elsewhere. Without it, Discover shows an "unavailable" state and the rest of the app keeps working.

```bash
npm test          # timing, ordering and resolution rules
npm run typecheck
npm run build     # type-check, bundle to dist/client, prepare the Sites worker output
```

## What is here

| Section | Behaviour |
| --- | --- |
| Library | Collections with picture covers; open one to see its wallpapers; full-size viewer with previous/next; add JPEG/PNG/WebP (25 MB, 60 megapixels) or MP4/MOV/WebM (200 MB) by button or by dropping files on the window; each file reports its own result. Right-click or the ⋯ button for actions. |
| Discover | Wallpapers and ready-made collections from the catalog server; Save / Saved; search; filters. "Live" lists animated wallpapers. A "For 2 displays" filter appears only when a second display is connected. |
| Rotation | A to-scale picture of your displays showing what is on them; Previous / Pause / Next; which collections take part; how often; shuffle. A Displays section (same on each / one across all / different on each) appears only with two or more displays. A battery option appears only when a live wallpaper is in use. |

A new install starts paused and never changes the desktop until Start is pressed.

**Resolution.** Each wallpaper is checked against the real pixels of the connected displays (`src/lib/fit.ts`). Rotation skips pictures that would look blurry; "one across all" stretches a picture only if it is large enough for the densest display and otherwise shows it on each display. Tiles too small for every display are marked "Low resolution", and the viewer says where a picture is too small.

## How it is put together

- `src/platform/` — the one boundary between interface and host. `types.ts` is the contract, `web.ts` the browser preview, `desktop.ts` the Mac app.
- `src/state/` — the library: collections, wallpapers, settings. Saved through the host: in a browser, the page's own storage (IndexedDB `walls-local`); in the Mac app, a `library-state.json` file beside the wallpapers. Data from the first prototype is migrated on load.
- `src/lib/schedule.ts`, `src/lib/rotation.ts`, `src/lib/fit.ts` — when the next change happens, which wallpaper comes next, and whether a wallpaper is large enough for a display. The Mac app's Rust core follows the same rules.
- `src/components/ui/` — buttons, dialogs, menus, select, switch on Radix primitives, styled from shared tokens in `src/styles.css`. See [NOTICE.md](src/components/ui/NOTICE.md) for component sources.
- `public/live.html` — the page that plays a video as a live wallpaper.

## Preview tools

In the browser only, the top bar has a **Preview tools** menu: pretend one, two or three displays are attached (to see the multi-display features appear), and switch between the three accent proposals (champagne, sage, clay). It is never shown in the Mac app.

## Sample images

`public/wallpapers/starter-*.jpg` are the starter library: twelve first-party originals from the catalog generator (nine at 3840 x 2400, three panoramas at 7680 x 2400), each with a small thumbnail. The other nine JPEGs are the 1672 x 941 sample images from the first prototype; new installs no longer get them, but libraries that already have them still point at these files.

`.openai/hosting.json`, `worker/`, `scripts/prepare-sites-build.mjs` and `tests/sites-worker.test.mjs` package the preview for static hosting and are kept as they were.
