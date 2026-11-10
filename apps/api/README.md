# walls — the catalog service

This is the small backend behind the app's **Discover** section. It answers
"what wallpapers exist, and where are their files?" It is read-only over HTTP:
there are no user accounts, uploads or sync. Content gets in through
command-line tools that you, the operator, run.

It is one program, `walls`, with a few commands:

| Command | What it does |
| --- | --- |
| `walls serve` | Runs the HTTP API (default `:8787`). |
| `walls seed` | Draws the 30 starter wallpapers, registers the live scenes in `seed/scenes/` that pass the scene check, and creates the curated packs. Safe to re-run. |
| `walls ingest <dir>` | Imports a folder of your own licensed images, videos and live scenes. |
| `walls scene check <dir>...` | Checks live scene folders against the scene standard. Reads files only. |
| `walls rm <id>` | Removes one item and its stored files. |
| `walls render` | Writes the starter artwork to a folder without touching the catalog (for tuning the generator). |

## Run it

From this folder (`apps/api`), with Go 1.25:

```sh
go run ./cmd/walls seed && go run ./cmd/walls serve
```

`seed` takes roughly ten seconds and about 1 GB of memory at its peak. Then try
<http://localhost:8787/v1/wallpapers>.

Everything the program writes goes under `./data` (git-ignored): the catalog
database `data/catalog.db` and, by default, the files in `data/objects/`.

Run the tests with `go test ./...`.

## Deployed

The catalog is live at **https://walls.utkarshjoshi.com**, on the Oracle server reached as `agent-server-oci`, behind Cloudflare. Release builds of the app use this address; development runs use `http://localhost:8787`.

To deploy, or to update the server after `walls seed` or `walls ingest`:

```sh
./deploy/deploy.sh
```

It builds `walls` for the server, takes a consistent copy of the local catalog in `data/`, uploads both, and restarts the service. The server ends up serving exactly the local catalog, so the routine is: change the catalog here, look at it, run the script.

What runs on the server, in `~/walls` (`deploy/compose.yaml`):

- `api`: the `walls` program, not root, with a read-only filesystem apart from its data folder, serving files from the server's own disk.
- `web`: Caddy on ports 80 and 443 in front of it. It holds a public certificate that it renews by itself.

Useful on the server: `cd ~/walls && sudo docker compose ps`, `sudo docker compose logs -f api`, `sudo docker compose down` to stop everything.

Not set up: the `walls-assets` bucket (files are on the server's disk; add bucket settings in `~/walls/walls.env` to move them), any backup of the server's copy (the catalog is rebuilt from this repository, so the repository is the source of truth), and edge caching (see below).

**Cloudflare caching.** Files are sent with a year-long cache header, but Cloudflare only caches by file extension and these addresses have none, so every request currently reaches the server. Adding a Cache Rule in Cloudflare for paths starting with `/v1/files/` ("eligible for cache", respect origin headers) would let Cloudflare serve them. Files carry `no-transform` so Cloudflare does not rewrite them: a scene must arrive byte for byte.

## Settings

Settings are environment variables. For convenience `walls` also reads a
`.env` file in the folder you run it from; real environment variables win.
Copy `.env.example` to `.env` to start.

| Variable | Default | Meaning |
| --- | --- | --- |
| `WALLS_ADDR` | `:8787` | Address the API listens on. |
| `WALLS_DATA_DIR` | `./data` | Folder for the database and local files. |
| `WALLS_STORAGE` | `local` | `local` (files on disk) or `s3` (a bucket). |
| `WALLS_S3_ENDPOINT` | | Bucket endpoint URL (see below). |
| `WALLS_S3_REGION` | | Bucket region. |
| `WALLS_S3_BUCKET` | | Bucket name (`walls-assets`). |
| `WALLS_S3_ACCESS_KEY` / `WALLS_S3_SECRET_KEY` | | Bucket credentials. |
| `WALLS_CORS_ORIGINS` | Vite dev server and Tauri | Comma-separated browser origins allowed to call the API. |
| `WALLS_PUBLIC_URL` | | If set, file links in responses are absolute, starting with this. |
| `WALLS_STORAGE_CAP_BYTES` | `7000000000` | Storage budget. `seed` and `ingest` refuse to go over it. |

## Storing files in the Oracle Cloud bucket

With `WALLS_STORAGE=local` the API streams files from disk itself. With
`WALLS_STORAGE=s3` the files live in a private bucket and the API answers file
requests with a redirect to a signed link that works for 15 minutes.

To point it at the `walls-assets` bucket:

1. In the Oracle Cloud console, open your **Profile → Customer secret keys**
   and generate a key. You get an access key and a secret key; the secret is
   shown once.
2. Find your Object Storage **namespace** (Bucket details page) and the
   bucket's **region** (for example `ap-mumbai-1`).
3. Put these in `apps/api/.env` yourself. The file is git-ignored; do not paste
   keys anywhere else:

   ```sh
   WALLS_STORAGE=s3
   WALLS_S3_ENDPOINT=https://<namespace>.compat.objectstorage.<region>.oraclecloud.com
   WALLS_S3_REGION=<region>
   WALLS_S3_BUCKET=walls-assets
   WALLS_S3_ACCESS_KEY=<access key>
   WALLS_S3_SECRET_KEY=<secret key>
   ```

4. Run `walls seed` (and any `walls ingest`) with those settings. That uploads
   the files to the bucket and writes `data/catalog.db`.

The database always stays on local disk, next to wherever `walls` runs. The
simplest setup is to run `seed`/`ingest` on the server itself. If you load
content from your laptop instead, copy `data/catalog.db` to the server
afterwards.

To check your keys before loading anything, run the storage self-test against
the real bucket. It writes and deletes one small object under
`catalog/selftest0000/`:

```sh
WALLS_TEST_S3_ENDPOINT=https://<namespace>.compat.objectstorage.<region>.oraclecloud.com \
WALLS_TEST_S3_REGION=<region> WALLS_TEST_S3_BUCKET=walls-assets \
WALLS_TEST_S3_ACCESS_KEY=... WALLS_TEST_S3_SECRET_KEY=... \
go test ./internal/storage -run TestS3Integration -v
```

Known limit: in `s3` mode a web page that reads file bytes with `fetch()` or a
canvas (rather than plain `<img>`/`<video>` tags) needs the bucket itself to
allow cross-origin requests, because the browser follows the redirect to
Oracle's host.

## Adding licensed content

Put the files in a folder and run:

```sh
go run ./cmd/walls ingest /path/to/folder
```

What it accepts:

- **Images**: JPEG, PNG or WebP, up to 25 MiB and 60 megapixels. The type is
  judged from the file's contents, not its extension.
- **Videos**: MP4, MOV or WebM, up to 200 MiB, stored untouched. A video needs
  a poster image beside it with the same name (`clip.mp4` + `clip.jpg`),
  because this service has no video decoder. Previews are made from the poster.

Every file must have an **author** and a **license**. They are never guessed:
a file without them is skipped. Supply details in either or both of:

- a sidecar next to the file, `sunrise.json` for `sunrise.jpg`;
- a folder-wide `manifest.json`:

  ```json
  {
    "defaults": { "author": "Ana Ruiz", "license": "CC BY 4.0", "category": "mountains" },
    "items": {
      "sunrise.jpg": { "title": "First Sun", "tags": ["warm", "dawn"], "sourceUrl": "https://example.com/first-sun" }
    }
  }
  ```

Fields: `title`, `category`, `tags`, `author`, `license`, `sourceUrl`. The
sidecar overrides the manifest entry, which overrides `defaults`. Without a
title, the file name is used ("misty_ridge.jpg" → "Misty Ridge"). Without a
category, images go in `other` and videos in `live`.

Only files directly in the folder are read. The one exception is live scenes:
a sub-folder that holds a `scene.html` is imported as a scene, if it passes the
check described under "Live scenes" below. The command prints one line per
file saying what happened and why. Running it again is
harmless: an item's id is a fingerprint of its bytes, so unchanged files are
recognised and skipped, and edited details are updated in place. The command
only exits with an error when nothing at all could be imported.

Remove something with `go run ./cmd/walls rm <id>`.

## Live scenes

A live scene is a wallpaper made of code: one web page, `scene.html`, that the
app draws behind the desktop icons. Because it is running code, every scene
must pass a check, "the gate", before this service will publish it. The rules
are written down in [`docs/16-live-scene-standard.md`](../../docs/16-live-scene-standard.md).

A scene is a folder with three files: `scene.html`, `poster.jpg` and
`meta.json`. The gate reads them and refuses the scene unless:

- **All three files are there.** `scene.html` is at most 512 KB, `poster.jpg`
  is a real JPEG of exactly 1920×1200, and `meta.json` says `"standard": 1`
  and gives a title, an author and a licence. Credits are never guessed.
- **It carries the standard's content policy, where a browser is sure to
  apply it.** That one line is what stops a scene from loading or sending
  anything. There must be exactly one policy tag in the whole file, with
  exactly the standard's text, and nothing may come before it except the
  doctype, `<html>`, `<head>` and `<meta charset="utf-8">`. The Mac app applies
  the same rule when a scene arrives, so a scene that passes here is not
  refused there.
- **It uses nothing on the standard's forbidden list**: outside scripts,
  links, frames and forms, `http:`/`https:`/`ws:` addresses, `fetch` and the
  other ways to talk to a server, `eval`, storage, cookies, leaving the page,
  or reaching the window above. Each one found is reported with its line.
  Words in comments do not count. The three fixed names needed to build SVG
  from script (`http://www.w3.org/2000/svg`, `.../1999/xlink`,
  `.../1999/xhtml`) are allowed, exactly as written and nothing longer.
- **It reads `location.hash` and handles `paused`**, which is how the app tells
  a scene its size and freezes it to save energy.
- **`meta.json` says how the scene lays out across displays**: `"layout"` is
  `"canvas"` (one picture continuing across them) or `"per-display"` (drawn
  whole on each, like a clock). The API passes it on as `layout` on the item,
  so the app can preview the scene the way it will really run.

To add a scene:

```sh
# 1. Make the scene folder. The kit (Python, in seed/scene-sources/) writes
#    scene.html around a shader or a page you supply, and captures the poster.
python3 seed/scene-sources/scenekit.py --help

# 2. Check it. Every reason for a failure is listed; nothing is changed.
go run ./cmd/walls scene check seed/scenes/<name>

# 3. Publish it, with everything else in seed/scenes/.
go run ./cmd/walls seed
```

`seed` (and `ingest`, for a scene kept in another folder) runs the same check
itself. A scene that fails is skipped with its reasons and the rest of the run
carries on. After `seed`, the published scenes are exactly the ones in
`seed/scenes/` that pass: a scene that was removed from the folder, or was
changed and no longer passes, is taken out of the catalog. A scene's title,
tags, colour, layout, author, licence and source link come from its
`meta.json`, and its id is a fingerprint of `scene.html`, so an approved scene
cannot change without becoming a different item. Editing only `meta.json`
keeps the id; the next `seed` or `ingest` updates the stored details in place.

What the gate does not judge. It checks that a scene cannot reach anything and
follows the layout contract. It does not check that a scene is calm, that it is
cheap to run, that it really stops when paused, or that it is any good. It
also reads the text as written: code that goes out of its way to disguise a
forbidden call would pass the scan, and is stopped by the policy when it runs,
not by the gate. So a person still opens each scene, watches it for a minute
on a real display, and reads its code before it is published.

## Building for the server

The program is pure Go (no C compiler needed), so it cross-compiles to the
Oracle ARM host from any machine:

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o walls ./cmd/walls
```

Copy the `walls` binary and the `seed/` folder to the server, set the `WALLS_*`
variables, then `./walls seed && ./walls serve`. A `Dockerfile` is included as
an alternative (non-root, no shell).

## About the starter artwork

`walls seed` draws its 30 wallpapers in code (`internal/artgen`): five styles
(soft gradients, horizons, dunes, contour maps, aurora), 24 at 3840×2400 and 6
panoramas at 7680×2400. Random choices come from fixed seeds, so every run
produces the same files and therefore the same ids.

"The same" holds for a given Go release and CPU type. Building with a
different Go version, or on Intel instead of ARM, can change a few bytes and
so the ids. If that happens, `seed` adds the new versions and removes the old
ones it created; your ingested content is never touched.

To look at the artwork while changing the generator:

```sh
go run ./cmd/walls render -out data/render -scale 0.5 -sheet 6
```

## The API in one screen

All responses are JSON with camelCase keys. Errors are always
`{"error":"<code>","message":"<sentence>"}`.

```
GET /v1/health                    {"status":"ok","storage":"local","items":35}
GET /v1/categories                categories that have items, by name
GET /v1/wallpapers                ?kind=image|live &category= &q= &wide=1 &cursor= &limit=1..60 (default 30)
GET /v1/wallpapers/{id}           one item
GET /v1/packs                     curated packs with up to three cover thumbnails
GET /v1/packs/{id}                a pack and its items
GET /v1/files/{id}/{variant}      variant: thumb | preview | original | poster
```

For a live scene, `original` is the scene's `scene.html` and `poster` its
`poster.jpg`.

Lists are ordered by a `rank` set when content is loaded, and paged with an
opaque `cursor`: pass back the `nextCursor` from the previous page until it is
`null`. A `limit` above 60 is treated as 60.
