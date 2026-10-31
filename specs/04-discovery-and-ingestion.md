# Discovery and ingestion specification

Status: deferred discovery specification. Covers FR-17–21 and FR-27. Do not build external adapters, public feed, or submissions to ship the core product; its starter folders use a small reviewed local seed set.

## Pinterest-style discovery

Later, provide an image-first browse-and-save feed. A saved image enters a chosen private collection; if that folder is connected, rotation picks it up automatically. Content may come from approved sources or users who explicitly submit an upload for public discovery. Private uploads never automatically enter the feed. Submission must record redistribution permission, provenance, and moderation approval.

## Shared catalog, private choices

Catalog rows are global; library membership, collections, hidden images, and interests are per user. Public browsing exposes only approved metadata and permitted thumbnails. Saving an item creates a private reference, not a duplicate global upload. Anonymous users can browse; uploads and saves require an approved account.

Start with a manually curated seed set of owned/permitted works. First automated candidate is a narrow Wikimedia Commons category/query feed with reviewed file-level rights. Initial admission policy: verified CC0/public-domain works; add CC BY when the implementation can preserve attribution and document allowed transformations. CC BY-SA and ambiguous/custom licenses need separate review. See [source research](../docs/02-market-research.md) and [Commons reuse guidance](https://commons.wikimedia.org/wiki/Commons:REUSE).

## Adapter contract

Each adapter implements `discover(cursor, interests, limit)`, `resolve(external_id)`, `fetch_authorized_asset(record)`, and optional `refresh_rights(record)`. Return a canonical source URL, stable external ID, creator, license URL/name, attribution, image dimensions, MIME/size hints, source policy version, and explicit capabilities: `thumbnail_allowed`, `store_allowed`, `redistribute_allowed`, `offline_allowed`, `derivatives_allowed`.

Source credentials remain on the server. Defaults deny unknown capabilities. A generic URL-fetch endpoint is not part of the public API. An adapter must identify approved hosts, API quotas, caching rules, attribution presentation, and withdrawal procedure before enablement. Robots rules and terms govern access; they do not establish copyright permission on their own.

## Pipeline

`discovered → rights_pending → fetch_approved → quarantined → normalized → moderation_pending → published`.

At any step reject, retry, quarantine, or remove with a reason. Store provenance before downloading; validate per-file rights rather than inheriting a blanket license from a domain. Use API/feed access when available; add HTML scraping only for an explicitly approved source and with bounded request rates. Respect Retry-After; default concurrency one per host and an operator-set daily intake cap (initially 100 candidates/day).

Deduplicate `(source, external_id)` before fetch and validated hashes after normalization. Perceptual similarity can be added later; do not silently collapse artist variants. Decode in a restricted worker with memory/pixel/time limits; remove metadata from served derivatives; no SVG, scripts, arbitrary ZIPs, or video in the initial pipeline.

SSRF protections apply at every redirect and DNS connection: allowlisted HTTPS hosts, reject loopback/private/link-local/multicast/metadata addresses in IPv4 and IPv6, pin validated addresses for the connection, preserve hostname verification, limit redirects, and enforce time/byte caps. Re-resolving an unvalidated host inside the fetch client would undo the check. Never forward user cookies or infrastructure credentials to source sites.

## Ranking without a recommendation service

Filter by publication state, compatible dimensions/orientation, source capabilities, and personal hides. Score explicit tag overlap first, then recent publication; cap consecutive entries from one source/creator. Use stable ID as tie breaker. Persist a query timestamp in the cursor so pagination is consistent. Show matched tags as the reason for recommendations. No identity graph, cross-site tracking, or scraped personal profiles. Optional similarity/KNN/clustering learning and prompt-based curation are separate later tracks; see [future intelligence](07-future-intelligence.md).

## Attribution and moderation

Every item carries a canonical source page, creator, license link, and any modification note. Preserve those in web detail, the Mac “About this wallpaper” panel, export metadata/sidecars, and download context. If a license requires attribution in a location this product cannot provide, do not admit it merely because attribution exists in the database.

Shared beta is SFW with a curated initial catalog. Users can hide personally or report rights/safety/quality issues. Moderator removal unpublishes the entry, blocks new grants/saves, marks relevant variants blocked in device state, invalidates affected plans, and schedules cache/object cleanup. Record decisions and source evidence for audit. An offline device learns at its next sync; already issued 15-minute grants may not be individually revocable.

Unknown rights means “not admitted,” not “probably safe.” Fan art and television stills require the same evidence as other content. Private uploads are isolated from publication; the uploader must have permission for their use, but the system does not automatically claim redistribution rights.

## Operational controls

Per-source enable/disable, maximum requests/day, maximum bytes/day, last successful cursor, current policy version, failure rate, and quarantine counts. Stop ingestion on repeated policy/auth failures; do not retry a permanent 403 indefinitely. A global catalog kill switch prevents new public publication while allowing private existing libraries to function.
