# Implementation roadmap

Dependency order matters more than dates. Effort ranges below assume one experienced maintainer working substantially full time; they are planning estimates, not commitments. Distribution, provider approval, and available time can extend them.

| Stage | Indicative effort | Deliverable | Exit evidence |
| --- | --- | --- | --- |
| 0. Feasibility | 2–4 days | Native Swift wallpaper/login/wake spike; OCI upload/download/signing spike; inventory both servers | Real Mac applies a local image across attached screens; wake behavior recorded; actual S3 operations work; resource assumptions corrected |
| 1. Foundation | 3–5 days | Go API/worker structure, PostgreSQL migrations/jobs, OIDC owner access, static web shell, local development setup | Authorized session; forbidden cross-owner request; durable job survives process restart; health/readiness checks |
| 2. Personal library | 4–7 days | Upload/normalize, thumbnails, tags, collections, explicit playlists, quota controls | Upload failures are visible; valid file becomes a selectable playlist item; duplicate saving does not create duplicate references |
| 3. Schedules and device protocol | 4–7 days | Schedule validation/preview, deterministic compiler, pairing, immutable manifests, object grants | Contract fixtures agree across Go/Swift; DST/overlap cases pass; schedule edit compiles without partial publication |
| 4. Mac companion | 5–8 days | Pairing UI, cache, atomic activation, timers, pause/skip/pin, wake/restart recovery | Network-disabled 72-hour trial; corrupted/expired download recovery; no blank wallpaper on failure |
| 5. Personal deployment | 2–4 days plus dogfood | Oracle services, TLS/domain, backup/restore, versioned release build | Restore drill succeeds; 30-day personal dogfood starts; resource measurements recorded |
| 6. Shared discovery beta | 1–3 weeks | Approved-source ingestion, public catalog, private saves/interests, moderation/takedowns, quotas | Reviewed seed catalog, source kill switch, withdrawal propagation, isolation/load checks |
| 7. Extensions | Re-estimate after beta | Per-display schedules, Android, richer curation, iOS feasibility | Platform-specific proof before committing feature parity |

The personal build is approximately 20–35 engineering days plus elapsed dogfood time. Stop after Stage 0 to revise the plan if platform or storage constraints contradict assumptions; no big architecture rewrite should be required just to test wallpaper application.

## First implementation slice

One owner, one Mac, three owned JPEGs, one ordered playlist, one hourly schedule, daily sync, and one authenticated object store. Demonstrate the entire loop before building discovery ranking or elaborate library UI.

## Quality gates

- Server: unit tests for scheduler/selection, database integration for ownership/jobs, API contract tests, and decoder failure fixtures.
- Web: one end-to-end upload → playlist → schedule → device-status flow; keyboard checks on core screens.
- Mac: integration test with local fixtures, checksum/cache interruption tests, real-machine wake/display experiments, energy measurement.
- Operations: deploy to a test environment, exercise bad migration/application rollback, restore backup, check ARM64 compatibility.
- Public beta: recheck source policies, verify attribution display/export, enforce quotas, test catalog withdrawal and account deletion.

The initial repository is planning only. No GitHub remote, cloud resources, paid services, or Apple distribution credentials are required to review these documents.
