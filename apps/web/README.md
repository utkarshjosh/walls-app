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
npm test          # timing and ordering rules (19 tests)
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

## How it is put together

- `src/platform/` — the one boundary between interface and host. `types.ts` is the contract, `web.ts` the browser preview, `desktop.ts` the Mac app.
- `src/state/` — the library: collections, wallpapers, settings. Saved locally (IndexedDB `walls-local`); data from the first prototype is migrated on load.
- `src/lib/schedule.ts`, `src/lib/rotation.ts` — when the next change happens and which wallpaper comes next. The Mac app's Rust core follows the same rules.
- `src/components/ui/` — buttons, dialogs, menus, select, switch on Radix primitives, styled from shared tokens in `src/styles.css`. See [NOTICE.md](src/components/ui/NOTICE.md) for component sources.
- `public/live.html` — the page that plays a video as a live wallpaper.

## Preview tools

In the browser only, the top bar has a **Preview tools** menu: pretend one, two or three displays are attached (to see the multi-display features appear), and switch between the three accent proposals (champagne, sage, clay). It is never shown in the Mac app.

## Sample images

The nine JPEGs in `public/wallpapers` are generated illustrative images from the first prototype, shown as starter collections. They are not a licensed catalog.

`.openai/hosting.json`, `worker/`, `scripts/prepare-sites-build.mjs` and `tests/sites-worker.test.mjs` package the preview for static hosting and are kept as they were.
