# Implementation roadmap

The core revision removes playlist editing, weekly rules, queue previews, and pin timers. Backend integrity and offline recovery remain necessary.

| Stage | Deliverable | Exit evidence |
| --- | --- | --- |
| 0. Feasibility | Swift wallpaper/login/wake and OCI access spikes; server inventory | Real Mac application and object grants work; capacity assumptions checked |
| 1. Foundation | API/worker, PostgreSQL, owner access, uploads, collection model | Private safe uploads, durable jobs, basic ARM deployment |
| 2. Effortless setup | Reviewed starter templates, tag mapping, idempotent onboarding, default rotation, pairing | User accepts folders and connects Mac without schedule configuration |
| 3. Live folder rotation | One/many folders, deduped shuffle/sequential, frequency presets, generated plans | Save/remove in connected folders changes rotation without a second action |
| 4. Reliable companion | Prefetch/cache, atomic activation, offline playback, Next/Pause, wake recovery | 72-hour offline trial; failed downloads preserve desktop; no routine maintenance |
| 5. Personal release | Oracle, TLS, signing plan, backup/restore, thirty-day dogfood | First-use trial and restore succeed; resources measured; experience stays simple |
| 6. Discovery later | Pinterest-style browse/save, approved sources, explicit user submissions, moderation | Safe shared catalog; a save into a connected folder reaches rotation |
| 7a. Taste learning later | Opt-in signals, similarity/KNN/clustering experiments | Benefit over tag baseline; disable/reset works |
| 7b. Prompt curation later | Natural language to reviewable collection/rotation proposal | Grounded authorized results, clear apply/undo, no calendar UI |
| 8. Platform expansion | Android; separate iOS feasibility | Real platform proof before feature promises |

No embeddings, vector database, model hosting, agent runtime, public scraper, or elaborate control screen is needed for stages 0–5. Intelligence tracks 7a and 7b are independent.

## First slice

One owner selects a starter folder, pairs a Mac, and gets hourly shuffled wallpaper. Add a second folder, confirm shared images are deduplicated, and save a new image into a connected folder. It joins rotation after sync without creating a playlist.

## Validation and estimates

Re-estimate after the feasibility spike and onboarding walkthrough. The previous 20–35-day estimate included removed features and is superseded; fewer controls do not eliminate native/offline engineering.

Server checks cover ownership, jobs, collection invalidation, traversal continuation, quota, and image safety. Web checks cover starter choice → pairing → automatic rotation and upload/save → connected folder. Native tests cover wake/displays, cache corruption, interruption, and quiet recovery. Operations still require a restore drill.

Discovery adds rights/moderation checks. Learning needs usefulness/privacy evidence; prompt curation needs authorization, grounding, review, and undo. Do not build those dependencies speculatively.
