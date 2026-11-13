# walls — the catalog service

This is the small backend behind the app's **Discover** section. It answers
"what wallpapers exist, and where are their files?" The catalog is read-only
over HTTP. Content gets in through command-line tools that you, the operator,
run.

It also does four jobs around the catalog, described under
[Accounts, downloads and the admin portal](#accounts-downloads-and-the-admin-portal):

- it lets people sign in to the app with a code sent to their email address
  (no passwords), and records what a signed-in person does with catalog
  wallpapers;
- it keeps the builds of the app and hands them to the website's download
  buttons;
- it counts page visits, downloads and fetches as plain daily totals;
- it shows all of that to the owner at `/admin`.

It is one program, `walls`, with a few commands:

| Command | What it does |
| --- | --- |
| `walls serve` | Runs the HTTP API (default `:8787`). |
| `walls seed` | Draws the 30 starter wallpapers, publishes the stills listed in `seed/stills/stills.json`, registers the live scenes in `seed/scenes/` that pass the scene check, and creates the curated packs. Safe to re-run. |
| `walls ingest <dir>` | Imports a folder of your own licensed images, videos and live scenes. |
| `walls scene check <dir>...` | Checks live scene folders against the scene standard. Reads files only. |
| `walls rm <id>` | Removes one item and its stored files. |
| `walls users` | Prints the people who have signed in to the app as CSV. Safe while the service is running. |
| `walls backup <file>` | Writes a complete copy of `records.db` to a new file. Safe while the service is running; the deploy script uses it. |
| `walls release <version> <platform> <file>` | Adds a build of the app to a release; with `-publish` the website offers it. Safe while the service is running. `deploy/release.sh` runs it on the server. |
| `walls signups` | Prints the earlier list of people who asked for a build (from the form the landing page used to have) as CSV. |
| `walls render` | Writes the starter artwork to a folder without touching the catalog (for tuning the generator). |

## Run it

From this folder (`apps/api`), with Go 1.25:

```sh
go run ./cmd/walls seed && go run ./cmd/walls serve
```

`seed` takes roughly ten seconds and about 1 GB of memory at its peak. Then try
<http://localhost:8787/v1/wallpapers>.

Everything the program writes goes under `./data` (git-ignored): the catalog
database `data/catalog.db`, by default the files in `data/objects/`,
`data/records.db` (users, what they did, the list of builds, daily totals,
admin sign-ins) and `data/releases/` (the builds themselves).

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
| `WALLS_ADMIN_PASSWORD` | | The owner's password for `/admin`, at least 12 characters. Empty switches the portal off. |
| `WALLS_SMTP_HOST` | | The mail relay that sends sign-in codes. Empty switches signing in off. |
| `WALLS_SMTP_PORT` | `587` | `587` upgrades the connection with STARTTLS; `465` is encrypted from the start. |
| `WALLS_SMTP_USER` / `WALLS_SMTP_PASSWORD` | | The relay's sign-in. Both or neither. |
| `WALLS_MAIL_FROM` | | The address codes come from, e.g. `Still <hello@example.com>`. Needed with a relay. |
| `WALLS_MAIL_TO_LOG` | | `1` writes each message to the server log instead of sending it. Development only. |

## Accounts, downloads and the admin portal

### Signing in

Signing in is optional for the people who use the app, and has no password.
There is no separate auth service: it is about 400 lines in this program
(`internal/httpapi/auth.go`) plus a mail relay you rent to send the code.

1. The app posts an address to `POST /v1/auth/start`. The server emails it a
   six-digit code that works once, for ten minutes.
2. The app posts the address and the code to `POST /v1/auth/verify` and gets a
   token back. The first time an address does this it becomes a user, so
   signing up and signing in are the same thing.
3. The app sends the token as `Authorization: Bearer <token>`. It lasts 180
   days from its last use. It is not a cookie, so a page on another site
   cannot make a signed-in request.

The server stores a hash of each code and each token, never the thing itself.
Guessing is bounded: five wrong tries use a code up, and codes are limited to
one every 45 seconds and five an hour for one address, twelve an hour from one
connection, and 200 an hour in total (each code is an email, which costs money
and can be used to pester someone).

**The mail relay.** Any provider that gives you an SMTP host, a user name and
a password works; set the `WALLS_SMTP_*` variables and `WALLS_MAIL_FROM` in
`~/walls/walls.env` on the server and restart `api`. Most providers make you
prove you own the sending domain first (a few DNS records). Nothing is ever
sent over an unencrypted connection: without TLS the message is refused.
Sending straight from the server is not practical on Oracle Cloud, which
blocks outgoing mail on port 25 unless Oracle grants an exemption. With no
relay set, `GET /v1/auth/status`
answers `{"signIn":false}` and the app does not offer signing in.

Locally, put `WALLS_MAIL_TO_LOG=1` in `apps/api/.env`: the code is written to
the server's log instead of being sent.

### What is kept

`data/records.db` holds, per user: the address, the country Cloudflare
reported, when they first signed in and were last seen, whether you have
blocked them, and your note. For each thing a signed-in app reports
(`POST /v1/events`) it holds the time, the kind, and what it happened to:

| Kind | Meaning | Kept with it |
| --- | --- | --- |
| `app_open` | The app was opened | its version, and whether it is the Mac app |
| `discover_open` | Discover was opened | |
| `search` | A search in Discover | the words |
| `view` | A catalog wallpaper was opened full size | its id |
| `save` / `remove` | A catalog wallpaper was saved to, or removed from, the library | its id |
| `set` | A catalog wallpaper was put on the desktop by hand | its id |
| `pack_open` / `pack_save` | A collection was opened or saved | its id |

Only catalog ids are accepted; a person's own pictures and videos are never
reported. Kinds this server does not know are dropped, so a newer app can talk
to an older server. A person can delete their account, and everything recorded
with it, from the app (`DELETE /v1/me`); you can do the same from the portal.

For people who are not signed in, the file holds only one number per day for
each of: landing page visits, downloads of each build, times the app opened
Discover, and times each wallpaper's full file was fetched. No IP address,
cookie or device identifier is stored anywhere.

The landing page's privacy page (`apps/site/privacy.html`) says all of this to
visitors. If what is kept changes, change that page in the same commit.

**The file is the only copy.** `catalog.db` is rebuilt from this repository and
replaced on every deploy. `records.db` and `data/releases/` are the opposite:
they exist only on the server and the deploy script never touches them. Each
deploy brings a complete copy of `records.db` back to
`data/backups/records-<time>.db` on your machine (it holds people's addresses;
`data/` is git-ignored). `walls backup <file>` does the same by hand.

### Downloads

The website's download buttons are fed from here. A **release** is one
version of the app with one **build** (file) per system. A release is a draft
until you publish it; the newest published release is the one the website
offers. The files are kept on the server's disk in `data/releases/`.

There are two ways to put a build on the website, and they do the same thing.

From this machine, with one command:

```sh
./deploy/release.sh 0.1.0 mac-arm64 ../desktop/src-tauri/target/release/bundle/dmg/Still_0.1.0_aarch64.dmg "The first public beta."
```

It copies the file to the server and runs `walls release -publish` there,
which adds the file to that version's release (starting the release if this is
its first file) and publishes it. Set `WALLS_DRAFT=1` to add the file without
publishing. It needs no password: like `deploy.sh`, it uses your ssh access to
the server.

Or in a browser: open `/admin`, **Releases**, start a release with its
version, choose the file and the system it is for, upload, publish.
Cloudflare's free plan refuses uploads over 100 MB that way; the command has no
such limit.

Either way, adding a file again for the same version and system replaces the
earlier one. Deploying the server does not publish anything: a build is on the
website only after one of these two.

How the website uses it: `GET /v1/releases/latest` lists the builds, and each
button links to `GET /v1/download/<platform>`, which adds one to that build's
count for the day and sends the browser to the file. `<platform>` is
`mac-universal`, `mac-arm64`, `mac-x64`, or just `mac` for whichever of those
exists, in that order. Windows and Linux names are accepted too
(`windows-x64`, `linux-x64`, ...), so offering one later is an upload, not a
code change; today the app itself only runs on macOS.

### The admin portal

Open `/admin`. It is off until you set a password:

```sh
# On the server, in ~/walls/walls.env (create it; never commit it):
WALLS_ADMIN_PASSWORD=<output of: openssl rand -base64 24>
# then: cd ~/walls && sudo docker compose up -d api
```

Locally, put the same line in `apps/api/.env` and open
<http://localhost:8787/admin>. There is one password, for you. It has nothing
to do with the accounts people make in the app: those cannot reach anything
under `/v1/admin`. Signing in sets a cookie that script cannot read and that
is only sent to `/v1/admin`; it lasts 14 days, and changing the password signs
every browser out. Eight wrong passwords from one connection lock sign-in from
it for fifteen minutes.

| Tab | What it shows, and what you can do |
| --- | --- |
| Activity | Totals and a 30-day chart each for page visits, downloads, new users, active users, Discover opened and wallpapers fetched; the most fetched wallpapers; the wallpapers saved by the most signed-in people; a log of what you did in the portal. |
| Users | Everyone who has signed in. Search, open one person to see what they did, keep a note, block (signs them out and keeps them out), remove (deletes them and everything recorded about them), download CSV. |
| Releases | Every release and its builds, with download counts. Start a release, upload and remove files, edit the notes shown on the website, publish or take back, delete. |
| Wallpapers | Everything in the catalog with size, how often it was fetched, and how many signed-in people saved it and set it; how often each collection was opened. Read-only: add and remove with `walls ingest` / `walls rm`, then deploy. |
| Early list | The people who left an address when the landing page had a form asking for one. The form is gone; the list is kept so you can still write to them. |

If Cloudflare starts caching `/v1/files/` (see above), fetch counts will only
include requests that reach this server. Download counts are not affected: the
counting address is never cached.

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

POST /v1/visits                   no body; adds one to today's page visits

GET /v1/releases/latest           the newest published release and its builds; 404 when there is none
GET /v1/download/{platform}       counts the download, then redirects to the file
GET /v1/releases/files/{id}/{name}  one build's file

GET  /v1/auth/status              {"signIn":true} when signing in is available
POST /v1/auth/start               {"email":"..."}  →  emails a code
POST /v1/auth/verify              {"email":"...","code":"123456"}  →  {"token":"...","user":{...}}
POST /v1/auth/signout             ends this sign-in                 (needs the token)
GET  /v1/me                       {"email":"...","createdAt":"..."} (needs the token)
DELETE /v1/me                     deletes the account and its records (needs the token)
POST /v1/events                   {"events":[{"kind":"save","subject":"<id>"}]}  (needs the token)

/v1/admin/...                     the admin portal's own API; needs the admin cookie
```

For a live scene, `original` is the scene's `scene.html` and `poster` its
`poster.jpg`.

Lists are ordered by a `rank` set when content is loaded, and paged with an
opaque `cursor`: pass back the `nextCursor` from the previous page until it is
`null`. A `limit` above 60 is treated as 60.
