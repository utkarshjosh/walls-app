# Wallpaper Server

A carefree wallpaper library: choose a few folders, connect your Mac, and let wallpapers change automatically. Browse and collect images while the app handles rotation, daily downloads, and offline playback. No ads or paid placement. Shared discovery and AI curation come later.

**Status:** planning repository, 31 October 2026. No application, server, client, or infrastructure has been implemented or deployed. Specifications below are proposed implementation decisions, not claims about working software.

## Start here

1. [Product brief](docs/01-product-brief.md) — idea, scope, user journeys, and success criteria.
2. [Existing products](docs/02-market-research.md) — free/ad-free options and the remaining gap.
3. [Functional requirements](docs/03-functional-requirements.md) and [non-functional requirements](docs/04-non-functional-requirements.md).
4. [Architecture](docs/05-architecture.md), [data model](docs/06-data-model.md), and [Oracle deployment and storage](docs/07-deployment-and-cost.md).
5. [Implementation roadmap](docs/08-roadmap.md) and [decisions and open questions](docs/09-decisions-and-questions.md).

## Product default

Pick interests or starter folders → connect the Mac → hourly shuffle begins. One or more collections feed rotation directly; saved additions join automatically after sync. Regular controls are folders, Shuffle/In order, and three frequency presets. No separate playlist, calendar, or future queue to manage. See [future intelligence](specs/07-future-intelligence.md) for the deferred taste-learning and prompt-curation tracks.

## Recommended design

| Part | Proposal |
| --- | --- |
| Server | Go modular monolith; separate API and worker processes from one codebase |
| Web | React + TypeScript + Vite, static assets served alongside the API |
| Data | PostgreSQL for collections, rotation preferences, and durable jobs |
| Images | Private OCI Object Storage through a narrow S3-compatible adapter |
| Mac companion | SwiftUI menu bar app, AppKit wallpaper API, Keychain, local disk cache |
| Hosting | Two existing Oracle ARM servers; proposed `walls.utkarshjoshi.com` |
| Scheduling | Server compiles a seven-day plan; Mac syncs daily and rotates locally |

The supplied capacity is **2 ARM cores / 12 GB RAM**. Whether this is per server or total, plus OS, disk, region, and tenancy allowances, still needs confirmation. The design does not require two full-size machines to serve one person.

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

The OpenAPI file intentionally covers the critical device-sync slice. The endpoint inventory specifies the rest for subsequent contract work. Example domains, IDs, hashes, and images are synthetic; examples do not download actual wallpapers.

## Delivery order

Prove native macOS rotation and OCI object access first. Deliver starter selection/upload → connected collections → automatic prefetch → offline rotation next. Add Pinterest-style discovery, including explicit user submissions, later. Taste learning and prompt-to-curation are separate optional additions after that. Android follows later; iOS automation remains a separate feasibility exercise.

See [validation notes](docs/10-validation.md) for checks on this planning package. No software license is selected yet; third-party image rights remain separate from any future code license.
