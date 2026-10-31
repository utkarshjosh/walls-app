# Acceptance scenarios

Planned tests, not tests that have passed. Use the IDs for release evidence. Baseline test set: owned JPEG/PNG fixtures, a malformed image, a decompression-bomb fixture, multiple accounts/devices, two displays, and controllable clock/network/storage faults.

| ID | Requirements | Scenario and expected result |
| --- | --- | --- |
| AC-01 | FR-01–06 | Upload three images into overlapping folders, connect both, and switch Shuffle/In order. The union contains each image once; deleting a folder preserves library files |
| AC-02 | FR-02, NFR-10 | Spoofed MIME, excessive bytes/pixels, truncated file, and path-like filename are rejected without uncontrolled memory use or object publication |
| AC-03 | FR-05, NFR-11 | Account B substitutes account A's asset, collection, rotation, device, manifest, or export ID; no metadata, bytes, or existence hints leak |
| AC-04 | FR-07–09 | At 22:00 preferred sync, client receives seven days of hourly entries; it makes no server call at individual changes |
| AC-05 | FR-07 | Frequency accepts only three presets; one/many folders work. Empty folders are skipped; when all are empty/disconnected, sync stops changes and retains the current image without substituting unrelated content |
| AC-06 | FR-07, NFR-07 | Fixed DST gap/fold fixtures with a pinned timezone database produce ordered unique UTC entries and one daily sync; detected timezone changes update the next plan at sync without asking for manual timezone setup |
| AC-07 | FR-09 | Compile identical source revisions/checkpoints twice; results match and cursor does not advance twice. Race a collection edit against compilation; stale output cannot become latest |
| AC-08 | FR-10 | Pairing expires, wrong code is throttled, approved challenge consumes once, another user cannot approve the wrong device silently; token never appears in URL/logs |
| AC-09 | FR-11 | Interrupt download at 50%, tamper checksum, or expire grant. Active plan remains usable; restart resumes/restarts safely; new grant avoids changing manifest |
| AC-10 | FR-11, FR-15 | Fill cache while staging replacement. Active/current files stay protected; client automatically requests a smaller candidate budget and retries, without partially activating or asking the user to manage the queue |
| AC-11 | FR-12, NFR-03 | Activate plan, disable all networking for 72 hours, restart app and sleep/wake; correct due items appear and no missed-event replay occurs |
| AC-12 | FR-12–14 | Pause, skip repeatedly, resume, change wallpaper externally, add/remove display. Defined local overrides persist; no fighting another app |
| AC-13 | FR-09–12 | Manifest 304 still fetches device state; blocked current image switches to fallback even if a replacement plan is compiling |
| AC-14 | FR-10, NFR-09 | Online revocation denies API/grants; offline client continues only with known local state until contact; rejected auth pauses automatic changes |
| AC-15 | FR-16, NFR-08/18 | Export contains only the owner's permitted files/provenance. Delete user, restore an older backup, replay deletion log; deleted data does not reappear publicly |
| AC-16 | FR-17–20 | Approved catalog items save into private collections and reach rotation if connected; unreviewed entries never publish; source retries do not duplicate items |
| AC-17 | FR-20, NFR-10 | Adapter redirects to loopback, private IPv6, cloud metadata, or DNS-rebound address; network request is denied at connection time |
| AC-18 | FR-21 | Moderator withdraws asset; public view/save/grants deny it, device state blocks it, fresh manifests exclude it, attribution/audit evidence remains |
| AC-19 | FR-22, NFR-17 | Concurrent uploads exceed reserved quota; only allowed bytes commit. Global storage/request circuit breaker stops intake without stopping cached playback |
| AC-20 | NFR-04–07/14 | Measure documented latency/resource targets, check keyboard/reduced-motion flows, record supported macOS/display combinations |
| AC-21 | NFR-08/15 | ARM64 deployment and clean restore from encrypted backup succeed within target; DB remains private; recovery keys work |
| AC-22 | FR-10–12 | Duplicate event submission is harmless; lost credential-rotation response recovers inside grace; outside grace client requests re-pair |
| AC-23 | FR-07–12 | Forward/backward clock jump or >5-minute server skew preserves current desktop, reports clock status, and never rapidly replays a queue |
| AC-24 | FR-25, NFR-19 | Choose interest tags or starter folders, pair, and reach hourly shuffle in three steps without schedule settings. Retry onboarding without duplicate folders; no-interest/general and upload paths work |
| AC-25 | FR-26 | Save/remove an image in a connected folder; next sync reflects it with no publish/playlist action. Unconnected folder changes have no effect |
| AC-26 | FR-13/15, NFR-19 | Thirty-day dogfood has no routine cache/queue maintenance. Brief offline/download failures recover quietly; only actionable exceptions request attention |
| AC-27 | FR-27 | Private upload stays private. Explicit discovery submission requires rights/moderation; rejected submissions never enter the public feed |
| AC-28 | FR-28 | Deferred: opt-in learning respects reset/disable, uses explicit feedback appropriately, and does not treat Pause/Next as dislike or expose private content |
| AC-29 | FR-29 | Deferred: prompt proposes a grounded permitted collection, clearly describes connected-folder changes, and supports Apply/Undo without a timeline editor |
| AC-30 | FR-06/09/15 | A large library advances the cache window across syncs; it never permanently favors the first N files. Refresh/replayed acknowledgment does not reset traversal or consume progress twice |

Core MVP gate: AC-01–15, AC-19–26 and AC-30 as applicable to implemented endpoints. Later discovery adds AC-16–18 and AC-27. Intelligence tracks separately require AC-28/29. Run native platform scenarios on real hardware; mocked AppKit calls cannot establish actual multi-display/Spaces behavior.
