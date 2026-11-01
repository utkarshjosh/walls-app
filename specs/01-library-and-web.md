# Shared collection interface specification

Proposed v0.3. The same React collection interface runs bundled inside the desktop app and optionally in a browser. Discovery is deferred. See the [design brief](../docs/12-design-brief.md).

## Product surface

Default screen: **Collections**, image grid, and Upload/Save. “All wallpapers” is a library view. **Wallpaper settings** is a small sheet. Account, connection, export, and troubleshooting live behind Settings. Add **Discover** only when the catalog phase ships.

No top-level Playlists, Schedules, Devices, or future-wallpaper timeline in MVP. Cache counts, manifest revisions, and queue state belong in diagnostics.

## Onboarding

1. **Find your starting point:** choose optional interests or starter folders. Fixed tag mappings suggest a few collections; “Surprise me” uses a general set, “Use my own” opens upload.
2. **Connect your Mac:** inside the signed-in desktop app, approve this Mac. Browser-first users can install/open it and complete short-code pairing.
3. **You're set:** show selected folders and defaults; start playback when the first plan is ready.

Desktop users start after install/sign-in; collection setup stays in the app. No mandatory cadence, mode, timezone, or sync configuration. Starter templates contain reviewed permitted images with provenance. Applying them transactionally creates private collection memberships without copying binaries. An idempotency key prevents duplicate folders on retries. Future template edits do not alter user folders; withdrawals still propagate. If a template is unavailable, offer another or private upload. No scraper, embedding model, or LLM is needed.

## Connected collections

A folder toggle says “Use for wallpapers.” Saves/uploads into enabled folders enter rotation after the next successful sync. Removing items excludes them at that refresh. A subtle “Updates automatically” message is enough; no plan publication step.

Wallpaper settings contains selected folders, Shuffle/In order, and Every hour/Every three hours/Every day. Defaults: hourly shuffle. Optional Morning/Night background-download preference is secondary.

“In order” traverses folders in persisted selection order and items in save order (oldest first, UUID tie breaker); initial selection follows visible starter/folder order. Duplicate assets appear once, at their first occurrence. No drag-to-build-playlist editor is required.

If one folder empties, use the others. If all selected folders are empty/disconnected, retain the visible wallpaper, stop automatic changes after the next sync, and show “Add a few wallpapers to get going” on the next visit. Never silently select unrelated folders or repeatedly notify the user.

## Browsing and upload

Use thumbnails, cursor pagination, simple filters, and keyboard-accessible controls. Reveal title, dimensions, source/creator, license, and download details on demand. Batch uploads target a collection, with inline folder creation.

Authenticated intent reserves quota → bounded stream into quarantine → idempotent completion queues processing → worker verifies format/pixels/decode, normalizes orientation, strips EXIF/GPS from derivatives, hashes and creates variants → ready image becomes eligible automatically.

Show concise failure/retry status per file. Release reserved quota on rejection; expire abandoned quarantine files after 24 hours. Private originals may preserve metadata for owner export with an explanatory note. Public derivatives are sanitized. JPEG/PNG only initially; declared MIME and filenames are untrusted.

## Quiet status

Healthy: “Connected” or no badge. Pending: “Updates automatically.” Temporary offline/download failure: keep cached playback and retry quietly. Actionable: “Reconnect your Mac,” “Add wallpapers,” or “Storage is full” after automatic recovery fails. Technical details are expandable.

Deleting a connected folder removes its association and uses the remaining folders. Removing the last selection stops changes after sync while retaining the visible image. Deleting an image removes collection memberships, with no hidden playlist copy.

Resource writes use ETags/If-Match; accepted edits regenerate plans internally. Sync now is a secondary local action; a browser request cannot wake a sleeping Mac.

## Desktop and browser parity

Shared components use an injected transport/platform interface. Desktop reads/writes go through a scoped native broker and user session; browser uses same-origin cookies. Device tokens cannot manage the library. Desktop offline mode permits cached browsing/playback and disables server writes with an explanation. No offline edit queue in MVP. Admin uses a separate browser entry, not consumer navigation.

## Discovery later

Pinterest-style visual browsing supports quick saves, remembering the last-used collection. A save into an enabled folder is sufficient for rotation. Explicit publication submissions allow user uploads into the common catalog only after review; personal uploads stay private by default.

Learning and prompt curation remain deferred: [future intelligence](07-future-intelligence.md).
