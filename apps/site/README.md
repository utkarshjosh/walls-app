# Still, the landing page

The public page for the app: one static page, no framework. TypeScript, plain CSS, and three.js for the hero.

```bash
npm ci
npm run dev        # http://localhost:5180
npm run build      # writes dist/
```

## What is on the page

| Part | What it does | Where |
| --- | --- | --- |
| Hero | A laptop and a monitor in 3D with one live wallpaper running across both. Scrolling goes through three states: overview, "one picture across all" (the whole picture is drawn faintly behind the displays), "a different one on each". | `src/hero.ts` |
| Facts | Four short statements about how the app behaves. | `index.html` |
| Live scenes | A real scene from the catalog running in a sandboxed frame, with a row to choose others. Only one runs at a time, and it pauses when scrolled away. | `src/main.ts` |
| Wallpapers | A strip of stills from the catalog. | `src/main.ts` |
| The app | Screenshots of the three tabs. | `public/media/app/` |
| Closing | Where to get it. | `index.html` |

All the wording is in `index.html`. The product name appears there and in `<title>`; it is still a working name, so changing it is a find-and-replace in that one file. The Mac App Store link does not exist yet: the three places that will point to it are the links with `href="#get"`.

## The hero runs real catalog scenes

The wallpaper on the 3D screens is not a copy made for the page. `src/main.ts` loads a scene's shader source straight from `apps/api/seed/scene-sources/<slug>/shader.glsl`, and `src/hero.ts` compiles it with the kit's shared functions (`runtime/shader-lib.glsl`). To show a different scene, change `HERO_SCENE` in `src/content.ts`. It must be a shader scene (one that has a `shader.glsl`).

The picture is placed by where each pixel is in the 3D room, not by each screen's own coordinates. That is what makes it line up across the gap between the two displays, the same way the app lays one wallpaper across a bounding box of all displays.

If WebGL cannot start, the page shows `public/media/hero.jpg` instead and everything else still works. With "reduce motion" turned on, the scene is drawn once and the entrance is skipped; scrolling still changes the three states because the person is driving it.

## Pictures and scenes

`public/media/` is generated, and committed so the page builds anywhere:

| Command | Writes | From |
| --- | --- | --- |
| `npm run media` | `scenes/`, `posters/`, `stills/`, and `src/media.json` | `apps/api/seed/scenes` and `apps/api/seed/stills` (needs `sips`, part of macOS) |
| `npm run capture` | `app/*.jpg`, `hero.jpg`, `og.jpg` | Headless Chrome looking at the running app preview and this page (the script's header says what to start first) |

Which scenes and stills appear, and in what order, is chosen by hand in `src/content.ts`.

## Why three.js is in the page but not in the wallpapers

A live scene is one self-contained file of at most 512 KB that may load nothing ([the scene standard](../../docs/16-live-scene-standard.md)); three.js alone is larger than that. Scenes are written as shaders through the kit instead, which is also cheaper to run all day behind a desktop. The page has no such limit, so the hero uses three.js, loaded as its own file after the text is on screen.

## Serving it

`apps/api/deploy/deploy.sh` builds this folder and uploads `dist/` next to the catalog; Caddy serves it at the root of the same address, with the catalog under `/v1`. The response headers (content policy, caching) are in `apps/api/deploy/Caddyfile`. Set `WALLS_SKIP_SITE=1` to deploy the catalog without touching the page.
