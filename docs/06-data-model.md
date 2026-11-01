# Data model

Logical design, not an executable migration. IDs are UUIDs, timestamps are UTC `timestamptz`, automatically captured device zones are IANA strings, and file sizes are integer bytes. Owner-scoped entities carry `user_id` explicitly.

| Entity | Important fields and relationships |
| --- | --- |
| users | `id`, OIDC issuer + subject unique pair, display name, role, quota, created/deleted timestamps |
| sessions | hashed opaque ID, user, client kind (browser/desktop/admin), scopes, expiry/revocation; CSRF secret for browser sessions; no OIDC token in the webview/browser |
| desktop_auth_handoffs | hashed single-use code, PKCE challenge, initiating app/redirect binding, expiry, approved user, consumed timestamp |
| storage_ledger / upload_reservations | aggregate/account committed and reserved bytes, bounded input/output allowance, expiry, cleanup state, reconciliation timestamp; transactional write admission |
| assets | `id`, owning user or catalog scope, original blob reference, normalized hash, dimensions, status, provenance, rights decision, created/removed timestamps |
| asset_variants | asset, kind (`thumbnail`, `desktop`), width, height, MIME, byte size, SHA-256, private object key, processing version; unique asset/kind/processing-version |
| source_records | adapter + external ID unique pair, canonical URL, creator, license URL/name, attribution text, checked_at, policy version, permitted operations, rights evidence |
| library_items | user + asset unique pair, title override, saved_at, hidden_at; links do not confer public rights |
| collections | user, name, created_at, revision; flat MVP hierarchy |
| collection_items | collection + library_item unique pair; same owner; saved_at plus asset ID defines stable traversal order; writes increment collection revision |
| tags / asset_tags | normalized tags and associations; distinguish source tags from personal tags so user edits never alter the global catalog |
| starter_templates / template_items | Operator-reviewed folder names, interest tags, permitted assets, provenance, revision; applying creates private collection references idempotently |
| schedules (internal rotation preferences) | One initial setup per user; selected collections, mode, cadence preset, sync window, captured timezone, revision; no weekly-rule JSON |
| schedule_collections | schedule + owned collection, selection position; unique membership and stable order; live references, not copied item lists |
| traversal_state | device, source revision, seed/cycle, sequence checkpoint, acknowledged manifest/entry; idempotent progress reconciliation |
| devices | user, name, platform, capabilities, assigned schedule, cache budget, last contact, latest and activated manifest IDs, revoked_at |
| device_credentials | device, hashed high-entropy token, issued_at, expires_at, revoked_at, last_used_at |
| pairings | hashed device code, hashed user code, expiry, approved user, consumed_at, attempt counters; approval and consumption atomic |
| manifests | device, version, schedule/collection/profile revisions, generated_at, valid_from/until, immutable JSON body, body hash, publication state |
| device_events | device + event_id unique pair, type, manifest, observed_at, received_at, compact payload; bounded retention |
| jobs / outbox | type, payload IDs, dedupe key, run_at, lease, attempts, status, error code |
| catalog_entries | asset, approved_at, moderation state, visibility, interest tags, source policy revision |
| preferences / hidden_items | user interests and personal exclusions; never public |
| reports / audit_events | reporter, asset, category, moderation decision, actor, timestamp; avoid sensitive raw image data in logs |

## Ownership and access

Library and collection requests always filter by the authenticated user. Use composite foreign keys or equivalent transactional constraints to enforce same-owner collection members and live schedule-to-collection references. A device token may only act on its own device and read assets explicitly referenced by that device's currently authorized manifests. Desktop user sessions can manage owned collections but cannot invoke admin endpoints. Admin requires a browser admin session and role. Do not accept a caller-supplied user ID as authority.

Public catalog metadata is read-only to anonymous users; full-file download grants require an account/device and a permitted asset. Private thumbnails also require authorization. Public thumbnail access is possible only after a separate catalog approval check.

## Deduplication

Compute SHA-256 after validating/normalizing the image. Deduplicate within an owner and within the public catalog first. Avoid cross-owner private deduplication responses, hashes, timing differences, or “already uploaded by another user” hints. The storage key is internal and does not double as an authorization token.

Saving a public catalog image adds a user reference; it does not copy the source record or original bytes. Withdrawing a catalog item prevents new downloads and schedules across all references. A rights withdrawal is different from one user deleting a personal bookmark.

## Deletion and garbage collection

Removing a collection removes its memberships and rotation association, preserving the library files. Deleting a library item removes its collection memberships, increments affected revisions, and requests new plans. If every selected folder is empty/disconnected, device state disables rotation after sync while preserving the visible wallpaper. No independent playlist copy retains removed items. Blob GC runs only after checking live library/catalog references and retained unexpired manifests. Obsolete manifests retain their referenced variants for at least their seven-day horizon plus a one-day safety margin, unless a withdrawal requires earlier denial.

Account deletion revokes sessions/devices immediately, removes private access immediately, and schedules live-data purge within seven days. Backups expire within thirty days; a restoration must replay deletion/withdrawal records before serving traffic. Backups of private originals are not public exports.

## Indexes and migrations

Start with owner/date indexes for library lists; unique owner/asset membership; schedule/collection position; source/external-ID; pending jobs by run_at; device/version; and searchable normalized tags. PostgreSQL text search is sufficient for the initial catalog. Use cursor pagination with stable `(created_at, id)` ordering, not unbounded offsets.

Migrations use expand → deploy → backfill → contract for destructive changes. Back up before schema changes; avoid long locks on media/job tables. Content hashes, object keys, and the blob inventory must remain recoverable together with the DB.
