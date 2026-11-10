# Live scene standard (version 1)

Written 2026-11-10. A **scene** is a live wallpaper made of code rather than video: one HTML file the app draws behind the desktop icons. This page is the contract a scene must meet to be published in Discover, how one is made from material found on the web without writing the plumbing by hand, and how the app keeps a scene from doing anything except draw.

Why a standard: the owner wants many more live wallpapers (a live clock, popular shader pieces) without hand-coding each one, and a scene is running code, so every one has to pass the same checks.

## What a scene is

A folder named after the scene (lowercase, hyphens), holding exactly three files:

| File | What it is |
| --- | --- |
| `scene.html` | The whole scene in one file: markup, styles and script inline. 512 KB at most. |
| `poster.jpg` | A 1920 x 1200 still of the scene. Shown in grids, and set as the ordinary desktop picture underneath the running scene. |
| `meta.json` | Who made it and how it is described (below). |

`meta.json`:

```json
{
  "standard": 1,
  "title": "Quiet Clock",
  "tags": ["clock", "minimal"],
  "color": "#1c2230",
  "author": "Still Originals",
  "license": "Still Original",
  "sourceUrl": null,
  "layout": "per-display"
}
```

`layout` says which of the two kinds of scene this is (rule 2): `"canvas"` for one picture across displays, `"per-display"` for a scene drawn whole in every window. The app relies on it: a per-display scene is never treated as stretched, and the app's own preview draws it complete on each display.

A scene that tells the time carries the tag `clock`. Its poster shows a frozen time, so the app does not put that poster on the desktop underneath the running scene (where the lock screen or Mission Control would show the wrong time); it leaves the existing desktop picture alone instead.

`author` and `license` are required, as for every other wallpaper: a scene adapted from someone else's work names them, gives the licence that allows commercial redistribution, and links the original in `sourceUrl`. Work whose licence forbids commercial use (the default on several shader sites) cannot be published, however good it looks.

## Rules for `scene.html`

### 1. It can reach nothing

The file opens with the doctype, `<html>`, `<head>` and `<meta charset="utf-8">`, and the very next thing is this exact line. Nothing else may come before it (no comment, title, style or script), the tag has only these two attributes, and there is no other policy anywhere in the file:

```html
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; base-uri 'none'; form-action 'none'">
```

The position matters: a policy that appears after other content, or inside a comment or template, can be ignored by the web engine, so both the gate and the app refuse it. The tag may be written with its attributes in either order, in any letter case and with single, double or no quotes; what must match exactly is the `content` value.

With that policy the scene can run its own inline script and styles and use pictures or fonts embedded as `data:` URLs. It cannot load or send anything: no network, no other files, no frames, no workers, no media.

The file must also not contain: `<script src>`, `<link>`, `<iframe>`, `<object>`, `<embed>`, `<form>`, `<base>`, `<a href>`, `http:`/`https:`/`ws:` URLs in code, `fetch`, `XMLHttpRequest`, `WebSocket`, `EventSource`, `sendBeacon`, `importScripts`, dynamic `import(`, `eval`, `new Function`, `localStorage`, `sessionStorage`, `indexedDB`, `document.cookie`, `window.open`, assignments to `location` (reading `location.hash` is how geometry arrives and is allowed), `navigator.` APIs other than `hardwareConcurrency` and `userAgent`, `Notification`, `postMessage`, or `top.`/`parent.` access. Most of these would fail under the policy anyway; forbidding them keeps a scene honest and reviewable.

Three addresses are allowed because they are names, not places a scene can reach: the W3C namespace strings `http://www.w3.org/2000/svg`, `http://www.w3.org/1999/xlink` and `http://www.w3.org/1999/xhtml`, which a scene needs in order to draw SVG from script. Addresses written without a scheme (`//host/path`) are not caught by the gate; the policy stops them when the scene runs.

### 2. It takes its geometry from the URL fragment

```
#cw=<canvas width>&ch=<canvas height>&vx=<window x>&vy=<window y>&paused=0|1
```

All in points. The canvas is the whole surface the wallpaper covers: one display normally, the bounding box of all displays when one wallpaper is stretched across them. `vx`,`vy` is where this window sits inside the canvas. Defaults when absent: the window's own size, 0, 0, not paused. The scene reacts to `hashchange` and `resize`.

A scene is one of two kinds, declared as `layout` in `meta.json` and honoured in how it uses these numbers:

- **Canvas scenes** (skies, gradients, landscapes) draw the part of the canvas their window covers, so the picture is continuous across displays.
- **Per-display scenes** (a clock, anything with a focal point that must not be cut by a bezel) ignore `cw`, `ch`, `vx`, `vy` and lay themselves out whole in their own window, so each display shows the complete scene.

An optional `&t=<seconds>` freezes the scene at that moment and draws one frame; it exists so a poster can be captured. For a canvas scene it is seconds into the loop; for a per-display scene it is seconds since local midnight.

### 3. It keeps time from the wall clock

Animation time comes from `Date.now()`, not from a counter, so separate windows stay in step and the scene looks the same after hours. Motion repeats over a fixed loop, or keeps the time value small, so precision does not degrade. A shader that never repeats is given a long loop with a slow dissolve across the join (the kit does this) rather than a visible jump.

### 4. It is calm and cheap

- A wallpaper, not a screensaver: slow, low contrast, nothing that flashes or pulls the eye, and it must leave menu-bar text and desktop icons readable.
- At most 30 frames a second. A full-window shader renders into a buffer no longer than 1600 pixels on its long side and lets the browser scale it up.
- `paused=1` means use as little energy as possible: stop continuous animation and timers and leave one finished frame. A scene that tells the time may still update once a minute while paused. `paused=0` resumes.
- When `document.hidden` is true, it stops as if paused.
- No CSS animations or transitions that run by themselves: they run at the display's full rate and ignore `paused`. Drive every change from the scene's own script.
- No sound, no input handling, no cursor changes, no selectable text, no scrollbars. `html` has a background colour so nothing flashes white (the body may be made transparent once the scene has drawn its own backdrop).

### 5. It degrades instead of breaking

If WebGL is unavailable, a shader scene shows a still gradient in its own colours. WebGL 2 may be used when a shader needs it, as long as the scene still falls back cleanly. A scene never throws to the console in normal use.

## Making a scene without writing the plumbing

The kit lives in `apps/api/seed/scene-sources/` and writes finished scenes into `apps/api/seed/scenes/`.

| You have | You run | You get |
| --- | --- | --- |
| A fragment shader in the common web style (`mainImage(out vec4 fragColor, in vec2 fragCoord)` with `iTime` and `iResolution`) | `scenekit.py wrap-shader` with the shader file, a slug, and the title, author, licence and source | A canvas scene: geometry, clock, frame cap, pause, fallback and policy are added around the shader |
| An idea for a scene made of ordinary HTML and CSS (a clock, a calendar) | `scenekit.py new-dom` with a slug | A per-display scene skeleton with the runtime already in place; you fill in what it draws |
| A finished scene folder | `scenekit.py poster` | `poster.jpg`, captured with a local Chrome |

A wrapped shader sees the canvas in points, not device pixels (`iResolution` is the size of the shared canvas). That keeps the picture identical across displays of different density, but a shader that depends on exact pixel sizes will look different from its original on the web.

Each scene's sources live in `scene-sources/<slug>/` (`scene.json` plus `shader.glsl` or `scene.html`); `scenekit.py build` rebuilds the finished files from them. Run `python3 scenekit.py --help` for the workflow.

Every scene, however it was made, then goes through the gate.

## The gate

`walls scene check <folder>` reads a scene folder and refuses it unless:

1. the three files are present and within their size limits, and `poster.jpg` is a real 1920 x 1200 JPEG;
2. `meta.json` is valid, with `standard: 1`, a title, an author, a licence and a `layout`;
3. the policy line is present, exact, in the position rule 1 requires, and is the only policy;
4. none of the forbidden constructs in rule 1 appears anywhere in the file;
5. the file reads `location.hash` and handles `paused` (so it can be laid out and frozen).

`walls seed` and `walls ingest` run the same check and skip a scene that fails, saying why. A passing check means the scene declares the policy that stops it reaching anything, and shows the basic signs of following the layout contract. It reads the code as written, so it cannot see through deliberately disguised code (the policy is what stops that at run time), and it does not check the frame rate, self-running CSS animations, whether the scene really stops when paused or hidden, whether the licence allows commercial use, or whether the scene is calm and good. A person still opens each scene, watches it for a minute on a real display, and reads the code before it is published.

## How the app contains a scene

Layers, outermost first. Each one holds even if the one before it is wrong.

1. **Only the catalog publishes scenes.** People cannot add their own scene files in version 1; the import dialog accepts pictures and videos only.
2. **The gate** above, at publishing time. A scene's id is a hash of its file, so a published scene cannot change afterwards without becoming a different item.
3. **The Mac app re-checks on arrival.** A downloaded `.html` file is kept only if it carries the exact policy line in the required position. Anything else is refused and never reaches the library. Only files named `.html` are ever run as scenes or shown in a preview frame.
4. **The policy itself**, enforced by the web engine while the scene runs: no network, and no access to other files, including the rest of the wallpaper library.
5. **The window it runs in** has no access to any app command, ignores the mouse and keyboard, and cannot navigate anywhere else. Previews inside the app run in a sandboxed frame.

What a hostile scene could still do if it got through review: waste energy, or show something unpleasant. It could not read files, contact a server, or act on the Mac. That residue is why review by a person stays in the process.

## Not in version 1

Scenes written or submitted by users, scenes that react to sound or the pointer, scenes that fetch live data (weather, calendars), and web pages as wallpapers. Each needs a wider policy and a different trust model.
