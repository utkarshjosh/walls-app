# Functional requirements

Revised 2026-11-03. **M** = core MVP, **D** = later discovery, **I** = later intelligence, **L** = extension. Retained IDs have revised meanings; this supersedes earlier playlist/time-window requirements.

| ID | Phase | Requirement and acceptance condition |
| --- | --- | --- |
| FR-01 | M | Google sign-in, initially invite-only; managed auth (Firebase Auth or Supabase Auth) chosen later. Use its supported flow with secure browser/native session handling; no custom password or OAuth system |
| FR-02 | M | Batch JPEG/PNG upload into a private collection with per-file outcomes; enforce 25 MiB / 60 MP limits |
| FR-03 | M | Image-first library with simple collection/text/tag filters; metadata and provenance on demand |
| FR-04 | M | Create/rename/delete flat folders; overlapping membership allowed; deleting a folder preserves library files |
| FR-05 | M | One saved asset per account; multiple collection references never duplicate its rotation weight |
| FR-06 | M | Connect one or more folders directly; choose Shuffle or In order without a separate playlist |
| FR-07 | M | Automatically create the initial rotation with hourly shuffle; offer hourly, three-hourly, daily presets; no rule builder/timeline |
| FR-08 | M | Automatically prefetch daily at night and catch up on wake. Optional Morning/Night preference is secondary; no precise time setup |
| FR-09 | M | Generate internal seven-day plans and refresh on changes; retain working playback through failures; queue details are not routine controls |
| FR-10 | M | Connect this Mac from the signed-in desktop app; browser-approved short-code pairing remains available; simple Connected/Needs attention status; secondary disconnect/re-pair |
| FR-11 | M | Verify bytes/hash/decode and atomically activate complete plans; failed staging preserves the working queue |
| FR-12 | M | Offline local rotation; wake applies the due image without replaying missed events |
| FR-13 | M | Primary Mac controls: Next, Pause/Resume, Open collection. Sync now/diagnostics secondary; no pin timers |
| FR-14 | M | Same image on attached displays through public APIs; per-display errors in diagnostics |
| FR-15 | M | Automatically bound/reduce cache candidates and repeat cached images when needed; request help only after automatic recovery fails |
| FR-16 | M | Private export, saved-item deletion, device revocation, and account deletion in secondary settings |
| FR-17 | D | Common Pinterest-style feed; quick saves to private collections; public browsing and authenticated saving |
| FR-18 | D | Interest filtering and Hide without requiring learned recommendations |
| FR-19 | D | Saving into a connected collection updates future rotation after sync, preserving attribution |
| FR-20 | D | Approved source adapters with provenance, bounded requests, deduplication, retries, and kill switch |
| FR-21 | D | Reports/moderator withdrawal deny publication/grants and notify connected clients of blocked assets |
| FR-22 | M | Internal per-user/install quotas; stricter public intake limits arrive with discovery; no routine capacity administration UI |
| FR-23 | L | Consider additional device setups after demand; granular scheduling/per-display editors are not committed features |
| FR-24 | L | Android extension and independently validated iOS export/automation feasibility |
| FR-25 | M | Optional interests or starter folders; general starter set/private upload also work. Automatically connect choices and apply defaults |
| FR-26 | M | Live folder membership invalidates future plans without republishing; retain current desktop until replacement activation |
| FR-27 | D | Public user-uploaded content requires explicit submission plus rights/moderation checks; private uploads never auto-publish |
| FR-28 | I | Optional taste learning using explicit likes/saves/hides/dislikes; opt-in for behavioral learning, reset/disable supported |
| FR-29 | I | Prompt to reviewable collection or rotation preferences via existing authorized operations; preserve the simple default experience |
| FR-30 | M | Full Collections experience inside the desktop app, sharing UI/API contracts with an optional browser client; no second management platform required |
| FR-31 | M | Separate role-protected admin web app for access, capacity, failed jobs, backup status, and intake pause |
| FR-32 | M | Direct Mac packaging with source/build instructions and explicit signing status; no App Store dependency or mandatory paid membership for development |

Acceptance scenarios: [specs/06-acceptance-scenarios.md](../specs/06-acceptance-scenarios.md). Starter content and direct folder rotation ship in MVP; public discovery and both intelligence tracks do not.
