# Library and web specification

Status: proposed v0.1. Covers FR-01–07, FR-10, FR-16–19, FR-21–22.

## Information architecture

Navigation: **Library**, **Collections**, **Playlists**, **Schedule**, **Devices**, **Discover**, **Settings**. Discovery is hidden until the approved catalog is enabled. The first-run view offers “Upload wallpapers,” followed by “Create playlist” and “Connect Mac.”

Library uses an image-first grid with cursor pagination, small thumbnails, selected count, and bulk add/remove controls. Filters include collection, tag, orientation, and minimum dimensions. Detail view includes original source, creator, license, resolution, size, saved collections, and download/export availability. A failed image has a retryable error, not an indefinitely spinning tile.

Collections behave like category folders but allow overlapping membership. Playlist editor exposes ordering, shuffle, count, approximate cache size, and missing/ineligible entries. Provide move-up/down buttons as a keyboard alternative to dragging. A playlist made from a collection is an explicit snapshot; later collection edits do not silently change it.

## Upload flow

1. Create an authenticated upload intent; validate quota, declared byte count, and accepted MIME. Reserve capacity transactionally so concurrent uploads cannot overrun the quota.
2. MVP streams the file through a bounded API upload endpoint into a quarantine object. Enforce the byte cap during streaming; neither Content-Type nor Content-Length is trusted alone. Direct browser signed uploads can follow after equivalent post-upload enforcement exists.
3. Complete upload with an idempotency key. The server queues processing and returns an asset ID with `processing` status.
4. Worker sniffs file type, checks dimensions, performs a constrained decode, normalizes orientation, removes EXIF/GPS from served derivatives, creates thumbnail/desktop variants, computes hashes, and publishes `ready` atomically.
5. Browser polls processing status with backoff; no WebSocket dependency. Rejected assets release reserved quota and quarantine files expire after 24 hours.

Originals remain private. They may preserve metadata for owner export, with a visible note; public originals must be sanitized or intentionally withheld. Only JPEG/PNG intake is required for MVP. HEIC/WebP conversion is a later explicit capability, not silently assumed.

## Schedule editor

User chooses a device, default playlist, rotation interval, and timezone. “Download new wallpapers daily at 22:00” appears separately from “Change wallpaper every 60 minutes.” Weekly windows can choose another playlist/interval. The editor shows a 24-hour timeline, estimated distinct downloads, and last device activation.

Saving updates desired configuration, not necessarily the Mac immediately. Show “Waiting for next sync” with the next preferred time. The Mac's Sync now command requests fresh configuration immediately; a browser “sync requested” flag alone cannot wake or reach a sleeping device.

Reject overlaps before saving and show the exact conflicting day/time. Empty playlists can be saved as drafts but cannot replace a valid active schedule without explicit warning; the compiler preserves the last valid device plan and reports the issue.

## Device page and client controls

Show last contact, last successful activation, next preferred sync, offline coverage end, cached bytes, client version, target displays, and recent error. Distinguish **synced**, **downloading**, **waiting for Mac**, **offline coverage low**, and **revoked**. A successful server response does not prove a plan was activated; use client acknowledgment.

Pairing confirmation displays the requesting device's name/platform and the entered code. Revocation immediately denies further server access, but cannot instantly erase files on an offline Mac. Disconnecting locally offers cache deletion and restores the previous wallpaper where possible.

## Discovery and interests

Shared catalog content is the same approved pool for everyone. An anonymous visitor chooses temporary interest filters; signed-in interests persist privately. “For you” begins with tags and hides, not inferred sensitive attributes or behavioral surveillance. “Save” requires login, preserves provenance, and does not publish the user's preferences.

Catalog cards show source/creator access and license details in the detail view. “Hide” is personal; “Report” enters moderation. A removed image cannot be newly saved or downloaded, even through an old public URL.

## Conflict and deletion behavior

Mutable resources carry a revision/ETag. Updates require `If-Match`; stale edits return 412 with current revision metadata. Reordering a playlist is one atomic replacement of the order, not many independent position mutations. Deleting a playlist referenced by a schedule returns 409 until it is replaced. Removing a collection only removes the folder/memberships. Deleting a saved wallpaper explains its affected playlists before confirmation.

Export provides a metadata JSON bundle plus permitted files and attribution sidecars. Imported external links without an offline-copy permission are not included as files. Private exports expire, require authentication, and are removed after 24 hours.
