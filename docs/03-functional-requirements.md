# Functional requirements

Status: proposed. Priorities: **M** = personal MVP, **B** = shared beta, **L** = later. Each row is independently reviewable and testable. Detailed behaviors live in the linked specifications.

| ID | Phase | Requirement and acceptance condition |
| --- | --- | --- |
| FR-01 | M | Owner signs in through one configured OIDC provider. Server sessions use secure cookies; browser mutations reject absent/invalid CSRF protection. Access is invite-only by subject allowlist initially |
| FR-02 | M | Upload JPEG/PNG images in batches; show per-file queued/processing/ready/failed status. Reject invalid type, >25 MiB, or >60 megapixels before uncontrolled decoding |
| FR-03 | M | View dimensions, size, title, tags, source, author, rights, and processing state. Search/filter own library by text, tag, collection, orientation, and resolution |
| FR-04 | M | Create, rename, and delete flat collections. One wallpaper can belong to several. Removing a collection does not delete the underlying saved items |
| FR-05 | M | Save the same wallpaper once per account and add it to multiple collections without copying its bytes. Unauthorized accounts cannot determine whether a private asset exists |
| FR-06 | M | Create/reorder a playlist using library items; choose ordered or seeded shuffle. A playlist uses a snapshot of selected members and records a revision |
| FR-07 | M | Create a device schedule with default playlist, IANA timezone, 15-minute to 24-hour rotation interval, and optional weekly windows; reject overlaps and preview the next 24 hours |
| FR-08 | M | Configure one preferred daily sync time (07:00 or 22:00 presets, arbitrary HH:MM allowed). It is independent of wallpaper-change frequency |
| FR-09 | M | Server compiles a versioned seven-day device plan on change and ahead of the next refresh. If not ready, serve the last valid plan or an explicit retry response; never an empty accidental replacement |
| FR-10 | M | Pair Mac via short-lived browser-approved code. Show device name, platform, last contact, activated manifest, cache coverage, and revocation control |
| FR-11 | M | Client downloads missing variants, checks size/hash/decode, atomically activates a complete plan, and retains a working fallback. A failed new download must not replace the existing wallpaper |
| FR-12 | M | Rotate from verified local files without network calls at each change. On wake apply only the currently due item; do not replay missed changes |
| FR-13 | M | Menu bar offers pause/resume, next, temporary pin, sync now, open web library, and status/error details. State survives restart |
| FR-14 | M | Apply the same image to all currently attached displays using public macOS APIs. Report per-display failures; do not claim all Mission Control Spaces or independent lock-screen support |
| FR-15 | M | Keep a bounded cache and show storage/coverage. If a requested plan exceeds the budget, report insufficient capacity and retain the active plan |
| FR-16 | M | Export personal metadata and owned/permitted image files with provenance. Delete saved items, account data, and device credentials through explicit actions |
| FR-17 | B | Everyone can browse a common approved discovery catalog without login; signing in enables saving and interest preferences. Private libraries never appear in it automatically |
| FR-18 | B | Filter shared discovery by explicit interests, orientation, minimum dimensions, and source; explain matching tags. Freshness and personal hide choices affect ordering |
| FR-19 | B | Save an approved catalog wallpaper to a private collection in one action; preserve original attribution and the source-policy snapshot |
| FR-20 | B | Scheduled adapters ingest only approved sources; deduplicate source IDs/hashes, enforce source limits, retry transient failures, and support an operator kill switch |
| FR-21 | B | Provide personal hide and abuse/rights reporting. Moderator can quarantine/remove content and prevent future grants/plans; connected clients receive tombstones |
| FR-22 | B | Enforce per-user and installation-wide storage, request, device, and import quotas before consuming resources |
| FR-23 | L | Support independent per-display playlists, nested folders, live collection-backed playlists, and multiple Macs after MVP contracts are stable |
| FR-24 | L | Reuse library and device protocol for Android with portrait variants and OS-appropriate scheduling. iOS starts with export/manual application or a validated Shortcuts workflow |

Acceptance scenarios and traceability: [specs/06-acceptance-scenarios.md](../specs/06-acceptance-scenarios.md). Important invariants: account isolation, no automatic publication, content rights before redistribution, and local rotation independent of daily sync.
