# Decision log and open questions

All architecture decisions are **proposed defaults** for implementation, not approvals to provision paid services or publish content. The requested local planning and Git initialization are complete when their artifacts/checks exist.

| ID | Decision | Rationale / revisit trigger |
| --- | --- | --- |
| ADR-001 | Go API/worker, shared React/TypeScript UI, Tauri 2 desktop + Rust native core | Revised 2026-11-01 for one desktop management experience; native/lifecycle feasibility gate and Swift webview fallback |
| ADR-002 | PostgreSQL jobs, no external queue initially | Durable transactions and one fewer service. Revisit when measured queue contention or throughput requires it |
| ADR-003 | Private OCI Object Storage with narrow adapter | Existing Oracle footprint and portability; verify actual API subset and allowances |
| ADR-004 | Once-daily pull + seven-day local plan | Desktop rotation survives network/server downtime; sleeping clients cannot be reliably pushed awake |
| ADR-005 | Desktop collection window plus native menu bar, no privileged system extension | Wallpaper application belongs in the logged-in user session; no root daemon or browser extension required |
| ADR-006 | Personal loop first; licensed discovery second | Proves value independently of a provider's API permission and limits |
| ADR-007 | Same image on attached displays for MVP | Reduces platform complexity; retain capability fields for future per-display support |
| ADR-008 | Explicit interests and simple ranking | Explainable and cheap; no recommendation model needed at initial scale |
| ADR-009 | Invite-only owner/pilot accounts | Bounds storage and moderation while tenancy capacity is established |
| ADR-010 | Live collections directly feed rotation | Supersedes snapshot playlists; save/remove flows into future rotation after sync |
| ADR-011 | Starter folders and automatic defaults | Interests map to curated seed content without AI; no schedule authoring required |
| ADR-012 | Three everyday controls | Selected folders, Shuffle/In order, frequency preset; remove weekly rules, queue previews, pin timers, and playlist editor from MVP |
| ADR-013 | Discovery and intelligence are deferred | Pinterest-style discovery accepts reviewed sources and explicit user submissions later; taste learning and prompt curation are independent optional tracks |
| ADR-014 | Shared UI bundled in desktop; separate admin web | Browser reuse without requiring two consumer management surfaces; remote content gets no native capability |
| ADR-015 | Private walls-assets, 7 GB intake / 8 GB hard cap | Owner reports root-compartment bucket and 8 GB rule, requires <10 GB; verify effective scope later |
| ADR-016 | Direct distribution, no Mac App Store | No-membership alpha first; signed/notarized public release if membership is funded; no Gatekeeper bypass |
| ADR-017 | Compact combined Collections reference with subtle motion | User prefers first/second concepts, rejects the spread-out third, and requests ongoing clean UI iteration |
| ADR-018 | Google sign-in; managed auth choice deferred | Firebase Auth or Supabase Auth at integration; avoid building a custom identity service |
| ADR-019 | Start on `agent-server-oci` | Mostly empty host is preferred for API, DB, worker and web; `oracle` already runs applications |
| ADR-020 | Motion for React with shared tokens | Restrained transitions, reduced-motion support, reusable components and documentation for human/LLM edits |

## Confirm before deployment

| Question | Current assumption | Work it blocks |
| --- | --- | --- |
| What are the exact resources, OS, and disk capacity of each SSH host? | 2-core ARM / 12 GB describes one server; `agent-server-oci` mostly empty, `oracle` has active apps | Verify planned single-host placement and resource limits |
| Same tenancy/region/VCN? Always Free or PAYG? Available storage/request limits? | Single host initially; tenancy/network details and effective quotas unconfirmed | Network configuration, quotas, cost commitment |
| Owner's macOS version and display/Spaces usage? | Proposed macOS 14+, same image on attached screens | Supported matrix and native acceptance tests |
| Firebase Auth or Supabase Auth project, and allowed owner identity? | Google sign-in confirmed; managed service choice/configuration deferred | Production sign-in integration, not UI prototyping |
| Confirm `walls.utkarshjoshi.com` and DNS management location | Proposed subdomain only | DNS/TLS deployment |
| When, if ever, fund Developer ID membership? | No membership now; direct ad-hoc alpha and source builds, signed/notarized wider release recommended | Frictionless public installation, not local development |
| Backup destination and recovery-key owner? | Separate permissions plus independent copy before beta | Recovery readiness |

## Product choices that can wait

Brand/name; explicit code license (MIT recommended for open-source reuse, not yet selected); any future decision to raise the currently fixed storage ceiling; later expansion of starter categories; shared-beta user count; Android-first versus an iOS export companion.

A small reviewed starter set is required before MVP onboarding ships; a broad catalog is not. Do not block the feasibility spike on branding or later-phase choices. No mandatory Unsplash/Pexels dependency, public sign-up, CDN, paid database, or automatic cloud upgrade is assumed.

The 2026-11-03 update keeps Google as the login choice and defers Firebase/Supabase selection. The earlier custom login/refresh endpoint sketch is no longer an implementation mandate: use a supported managed-auth flow when integration starts. Device sync authorization and separate admin access remain necessary. The sync OpenAPI is still partial.

The v0.2 document/API draft revises previously unreleased contracts: collection references replace playlist references, and rotation preferences replace rule windows. No deployed client or database migration is claimed.
