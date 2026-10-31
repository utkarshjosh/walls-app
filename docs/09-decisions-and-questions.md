# Decision log and open questions

All architecture decisions are **proposed defaults** for implementation, not approvals to provision paid services or publish content. The requested local planning and Git initialization are complete when their artifacts/checks exist.

| ID | Decision | Rationale / revisit trigger |
| --- | --- | --- |
| ADR-001 | Go API/worker, React/TypeScript web, Swift native Mac | Small backend footprint with direct Mac integration. Revisit language choice if maintainer expertise makes another stack substantially faster |
| ADR-002 | PostgreSQL jobs, no external queue initially | Durable transactions and one fewer service. Revisit when measured queue contention or throughput requires it |
| ADR-003 | Private OCI Object Storage with narrow adapter | Existing Oracle footprint and portability; verify actual API subset and allowances |
| ADR-004 | Once-daily pull + seven-day local plan | Desktop rotation survives network/server downtime; sleeping clients cannot be reliably pushed awake |
| ADR-005 | Native menu bar application, no privileged system extension | Wallpaper application belongs in the logged-in user session; no root daemon or browser extension required |
| ADR-006 | Personal loop first; licensed discovery second | Proves value independently of a provider's API permission and limits |
| ADR-007 | Same image on attached displays for MVP | Reduces platform complexity; retain capability fields for future per-display support |
| ADR-008 | Explicit interests and simple ranking | Explainable and cheap; no recommendation model needed at initial scale |
| ADR-009 | Invite-only owner/pilot accounts | Bounds storage and moderation while tenancy capacity is established |
| ADR-010 | Live collections directly feed rotation | Supersedes snapshot playlists; save/remove flows into future rotation after sync |
| ADR-011 | Starter folders and automatic defaults | Interests map to curated seed content without AI; no schedule authoring required |
| ADR-012 | Three everyday controls | Selected folders, Shuffle/In order, frequency preset; remove weekly rules, queue previews, pin timers, and playlist editor from MVP |
| ADR-013 | Discovery and intelligence are deferred | Pinterest-style discovery accepts reviewed sources and explicit user submissions later; taste learning and prompt curation are independent optional tracks |

## Confirm before deployment

| Question | Current assumption | Work it blocks |
| --- | --- | --- |
| Is 2-core ARM / 12 GB RAM per VM or total? What OS, disks, and existing workloads? | Fit core services inside combined capacity; bounded worker | Exact service placement and resource limits |
| Same tenancy/region/VCN? Always Free or PAYG? Available storage/request limits? | Private connection possible; allowances unconfirmed | Network configuration, quotas, cost commitment |
| Owner's macOS version and display/Spaces usage? | Proposed macOS 14+, same image on attached screens | Supported matrix and native acceptance tests |
| OIDC provider and allowed owner identity? | One provider; allowlist; no password system | Production sign-in configuration |
| Confirm `walls.utkarshjoshi.com` and DNS management location | Proposed subdomain only | DNS/TLS deployment |
| Apple Developer membership/distribution plan? | Local developer build first, signed/notarized direct download for others | Public Mac distribution, signing cost |
| Backup destination and recovery-key owner? | Separate permissions plus independent copy before beta | Recovery readiness |

## Product choices that can wait

Brand/name; code license (private, permissive, or copyleft); willingness to fund storage above free limits; later expansion of starter categories; shared-beta user count; Android-first versus an iOS export companion.

A small reviewed starter set is required before MVP onboarding ships; a broad catalog is not. Do not block the feasibility spike on branding or later-phase choices. No mandatory Unsplash/Pexels dependency, public sign-up, CDN, paid database, or automatic cloud upgrade is assumed.

The v0.2 document/API draft revises previously unreleased contracts: collection references replace playlist references, and rotation preferences replace rule windows. No deployed client or database migration is claimed.
