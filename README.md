# Wallpaper Server

A carefree wallpaper library: choose a few folders, connect your Mac, and let wallpapers change automatically. Browse and collect images while the app handles rotation, daily downloads, and offline playback. No ads or paid placement. Shared discovery and AI curation come later.

**Status:** planning repository, 3 November 2026. No application, server, or client has been implemented or deployed. The owner has created the private OCI bucket; its integration is untested. Specifications below are proposed implementation decisions, not claims about working software.

## Start here

1. [Product brief](docs/01-product-brief.md) — idea, scope, user journeys, and success criteria.
2. [Existing products](docs/02-market-research.md) — free/ad-free options and the remaining gap.
3. [Functional requirements](docs/03-functional-requirements.md) and [non-functional requirements](docs/04-non-functional-requirements.md).
4. [Architecture](docs/05-architecture.md), [data model](docs/06-data-model.md), and [Oracle deployment and storage](docs/07-deployment-and-cost.md).
5. [Implementation roadmap](docs/08-roadmap.md) and [decisions and open questions](docs/09-decisions-and-questions.md).
6. [Unified desktop and direct distribution](docs/11-desktop-and-distribution.md), [Collections design brief](docs/12-design-brief.md), and [motion/iteration guide](docs/13-motion-and-ui-iteration.md).

## Product default

Pick interests or starter folders → connect the Mac → hourly shuffle begins. One or more collections feed rotation directly; saved additions join automatically after sync. Regular controls are folders, Shuffle/In order, and three frequency presets. No separate playlist, calendar, or future queue to manage. See [future intelligence](specs/07-future-intelligence.md) for the deferred taste-learning and prompt-curation tracks.

## Recommended design

| Part | Proposal |
| --- | --- |
| Server | Go modular monolith; separate API and worker processes from one codebase |
| Interface | Shared React + TypeScript + Vite collection UI, bundled in desktop and reusable in browser; separate admin web entry |
| Data | PostgreSQL for collections, rotation preferences, and durable jobs |
| Images | Existing private OCI `walls-assets`; proposed 7 GB intake / 8 GB hard ceiling, within owner's <10 GB constraint |
| Mac app | Tauri 2 shell with bundled collection UI, Rust background core, AppKit adapter, Keychain, local cache |
| Hosting | Prefer `agent-server-oci` for all initial services; `oracle` hosts existing apps; proposed `walls.utkarshjoshi.com` |
| Scheduling | Server compiles a seven-day plan; Mac syncs daily and rotates locally |

The supplied **2 ARM cores / 12 GB RAM** describes one server. Two hosts are available: `oracle` has active applications; `agent-server-oci` is mostly empty and is the preferred initial deployment target for all core services. Exact inventory for each host remains to be checked.

Google sign-in is selected; Firebase Auth versus Supabase Auth will be chosen during authentication integration. Start UI iteration with local sample state, not a custom identity system. Keep the combined compact Collections reference, with subtle Motion for React transitions and shared UI tokens.

Users manage collections in the desktop app; a second management website is optional. Admin remains on the web. Direct Mac distribution avoids the App Store; a no-membership alpha has Gatekeeper friction, while Developer ID signing/notarization is the recommended later public release path. No paid membership is assumed for development.

## Implementation contracts

- [Library and web experience](specs/01-library-and-web.md)
- [Schedule compiler and sync protocol](specs/02-scheduling-and-sync.md)
- [macOS companion](specs/03-macos-client.md)
- [Discovery and ingestion](specs/04-discovery-and-ingestion.md)
- [API conventions and endpoint inventory](specs/05-api.md)
- [OpenAPI: device-sync vertical slice](specs/openapi.yaml)
- [Rotation preferences JSON Schema](specs/schemas/schedule.schema.json)
- [Manifest JSON Schema](specs/schemas/manifest.schema.json)
- [Example schedule](specs/examples/schedule.json) and [example manifest](specs/examples/manifest.json)
- [Acceptance scenarios](specs/06-acceptance-scenarios.md)
- [Admin web](specs/08-admin-web.md)
- [Storage budget and deferred integration checks](specs/09-storage-budget-and-integration.md)

The OpenAPI file intentionally covers the critical device-sync slice. The endpoint inventory specifies the rest for subsequent contract work. Example domains, IDs, hashes, and images are synthetic; examples do not download actual wallpapers.

## Delivery order

Prove native macOS rotation and OCI object access first. Deliver starter selection/upload → connected collections → automatic prefetch → offline rotation next. Add Pinterest-style discovery, including explicit user submissions, later. Taste learning and prompt-to-curation are separate optional additions after that. Android follows later; iOS automation remains a separate feasibility exercise.

See [validation notes](docs/10-validation.md) for checks on this planning package. No software license is selected yet; third-party image rights remain separate from any future code license.
