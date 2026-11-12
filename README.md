# Still (working name) — wallpapers that change on their own

A Mac app that keeps your desktop fresh: collect wallpapers, pick which collections take turns, and it rotates them for you. Connect a second display and it can stretch one wallpaper across both. It also plays live (animated) wallpapers.

**Direction, changed by the owner on 2026-11-12:** the app is free, in beta (0.x), and downloaded straight from the website. Signing in is optional, by a code sent to an email address; what signed-in people save and set is recorded so Discover can suggest wallpapers later. The earlier target was a paid app on the Mac App Store with no accounts; [the first-launch scope](docs/15-first-launch-scope.md) lists what that changes. How to announce it is in the [launch kit](docs/17-launch-kit.md).

**Status, 2026-11-12:** a working beta exists in four parts. None of the 2026-11-12 work is deployed yet.

| Part | Where | State |
| --- | --- | --- |
| Interface (React, TypeScript) | [apps/web](apps/web/README.md) | Library, Discover and Rotation work; runs in the Mac app and as a browser preview |
| Catalog server (Go, SQLite, object storage) | [apps/api](apps/api/README.md) | Serves Discover. Also: sign-in by emailed code, per-user activity records, the app's downloadable builds, and the owner's admin portal at `/admin` (users, releases, activity, wallpapers). **The catalog is deployed at https://walls.utkarshjoshi.com** (on `agent-server-oci`, behind Cloudflare); sign-in, downloads and the portal are **not deployed yet**, and sign-in needs a mail relay configured first |
| Mac app (Tauri 2, Rust) | [apps/desktop](apps/desktop/README.md) | Native shell: sets wallpapers per display, spans displays, plays live wallpapers, rotates from the menu bar. Builds for Apple silicon on this machine; an Intel or universal build needs the GitHub workflow or `rustup`. Not signed with an Apple Developer ID |
| Landing page (TypeScript, three.js) | [apps/site](apps/site/README.md) | One static page with a 3D hero that runs real catalog scenes, a download button that picks the build for the visitor's computer, and a privacy page. Built and previewed locally; **not deployed yet** |

What is deliberately **not** being built before launch, and what still blocks launch, is in [docs/15-first-launch-scope.md](docs/15-first-launch-scope.md). Read that before adding features.

## Run it

```bash
# 1. Catalog server (Discover) on http://localhost:8787
cd apps/api && go run ./cmd/walls seed && go run ./cmd/walls serve

# 2a. Browser preview on http://localhost:5173
cd apps/web && npm ci && npm run dev

# 2b. Or the Mac app (starts the interface itself)
cd apps/desktop && npm install && npm run dev
```

The documents below record how the plan got here. Where they disagree with the first-launch scope, the scope wins.

## Start here

1. [Product brief](docs/01-product-brief.md) — idea, scope, user journeys, and success criteria.
2. [Existing products](docs/02-market-research.md) — free/ad-free options and the remaining gap.
3. [Functional requirements](docs/03-functional-requirements.md) and [non-functional requirements](docs/04-non-functional-requirements.md).
4. [Architecture](docs/05-architecture.md), [data model](docs/06-data-model.md), and [Oracle deployment and storage](docs/07-deployment-and-cost.md).
5. [Implementation roadmap](docs/08-roadmap.md) and [decisions and open questions](docs/09-decisions-and-questions.md).
6. [Unified desktop and direct distribution](docs/11-desktop-and-distribution.md), [Collections design history](docs/12-design-brief.md), and [motion/iteration guide](docs/13-motion-and-ui-iteration.md).
7. [Current component foundation](docs/14-component-foundation.md) and [component catalog](docs/design/component-catalog.json) — approved reuse approach, source links, design principles and integration checks.
8. [First-launch scope](docs/15-first-launch-scope.md) — what ships first, the list of things not to build yet, and what blocks launch. Its opening section records the 2026-11-12 change of direction.
9. [Live scene standard](docs/16-live-scene-standard.md) — what a live wallpaper made of code must be, how to make one from web material, and how the app contains it.
10. [Launch kit](docs/17-launch-kit.md) — where to announce the beta, in what order, with the words ready to paste.

## Earlier product default (planning history)

Pick interests or starter folders → connect the Mac → hourly shuffle begins. One or more collections feed rotation directly; saved additions join automatically after sync. Regular controls are folders, Shuffle/In order, and three frequency presets. No separate playlist, calendar, or future queue to manage. See [future intelligence](specs/07-future-intelligence.md) for the deferred taste-learning and prompt-curation tracks.

## Earlier recommended design (planning history)

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

Google sign-in is selected; Firebase Auth versus Supabase Auth will be chosen during authentication integration. Start UI iteration with local sample state, not a custom identity system. Use the current component-first UI foundation, with shared tokens and subtle Motion for React transitions. The earlier collection-shelf mock is historical context; the latest component selections and minimal-information principles are recorded in the component foundation document.

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
