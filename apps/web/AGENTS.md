# Prototype Instructions

Run the local server yourself and open the preview in the browser available to this environment. Do not give the user server-start instructions when you can run it.

Before making substantial visual changes, use the Product Design plugin's `get-context` skill when the visual source is unclear or no longer matches the current goal. When the user gives durable prototype-specific design feedback, preferences, or decisions, record them in `AGENTS.md`.

When implementing from a selected generated mock, treat that image as the source of truth for layout, component anatomy, density, spacing, color, typography, visible content, and hierarchy.

Build app UI in `src/`. Keep `.openai/hosting.json`, `worker/index.js`, `scripts/prepare-sites-build.mjs`, and `tests/sites-worker.test.mjs` intact so the same local prototype can be handed to Sites. Before a Sites handoff, run `npm run build` and `npm run test:sites`; the build must leave `dist/client/index.html`, `dist/server/index.js`, and `dist/.openai/hosting.json`.

## Wallpaper product direction

Use the combined compact collection-shelf reference in ../../docs/design/collections-refinement.png. Keep a light neutral shell and restrained green accent. Motion stays subtle, respects Reduced Motion, and never animates gallery layout. Everyday rotation controls are folders, Shuffle/In order, and hourly/three-hourly/daily cadence. No timeline, AI/discovery navigation, metrics dashboard, or admin UI in this consumer slice. Google authentication and OCI/native integration are deferred; never present local preferences as working desktop integration. Keep shared tokens and components easy to evolve.
