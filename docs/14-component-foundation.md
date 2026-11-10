# Component foundation and source catalog

Reviewed November 8, 2026. Scope: select reusable components for the wallpaper desktop app from the five supplied libraries, replacing further freehand UI exploration with a component-first implementation.

## Decision and current design direction

The owner approved the component-first approach and asked to commit the selected pieces and research into this repository on November 8, 2026. Start from existing component source, tune shared tokens, and judge a small working slice before expanding the app. Do not continue generating whole-screen concepts as the default development loop.

- The product has three jobs: Library (local and/or synced wallpaper collections), Discover (individual community wallpapers and curated collections with clear Save/Saved behavior), and Rotation (one or more collections, shuffle, hourly/custom interval).
- Keep Library focused on browsing. No bottom player strip, routine sync-success message, persistent rotation status, duplicated collection shortcuts or decorative subtitle. Surface metadata in detail views and actionable errors where recovery is possible.
- Keep primary navigation labels visible. Provide Previous wallpaper, Pause and Next on Rotation; Previous follows actual displayed history even when shuffle is enabled. Keep application Back navigation separate.
- The owner rejected the blue/cyan accent. Dark neutral surfaces with a restrained nonblue accent are the current exploration. Champagne, sage and clay remain proposals; no final accent or generated mock was selected. “Still” is a working name only.
- Preserve local data, import validation and the current scheduling behavior while changing the UI. This research does not implement Discover, cloud sync, custom intervals or native wallpaper control, and it does not replace their backend contracts.

The [machine-readable component catalog](design/component-catalog.json) records the selected sources, trial status and known integration work.

## Foundation

Use **Shadcn Space's Radix variants as the main UI foundation**. Evaluate Space UI's Adaptive Density Grid with our own images. Treat Planes Photo Lightbox as an optional paid addition. Keep typography, spacing, radii and color tokens shared across every imported piece.

The [Shadcn Space component catalog](https://shadcnspace.com/components) provides ordinary controls and supports both Radix UI and Base UI. The existing frontend already uses React, Vite, Radix Dialog and Motion, so the Radix path avoids introducing a second primitive system for routine controls. Its current package manifest does not include Tailwind; that foundation and shared utility aliases will need setup before importing most examples.

## Concrete picks

| Product need | Actual component | Recommendation |
| --- | --- | --- |
| Library / Discover / Rotation navigation | [Shadcn Space Tabs 05 — Underline](https://shadcnspace.com/components/tabs) | Use its restrained visual treatment and primitive components; keep all three labels visible. |
| Wallpaper/collection grid | [Space UI Adaptive Density Grid](https://www.spaceui.one/components/adaptive-density-grid) | Trial with local wallpaper assets. Use a stable grid; hide density controls initially and disable reflow animation. |
| Full-size wallpaper preview | [Planes Photo Lightbox](https://useplanes.com/components/photo-lightbox) | Strong optional paid choice. Clear previous/next/close controls and minimal image framing. |
| Import | [Shadcn Space File Upload 01](https://shadcnspace.com/components/file-upload) | Free-listed starting point for a dedicated import dialog. Add existing validation/persistence, rather than treating the demo as a real upload service. |
| Collection and wallpaper actions | [Shadcn Space classic Context Menu](https://shadcnspace.com/components/context-menu) | Use the conventional menu, not the radial bubble variant. Also expose actions through an accessible menu button in the detail view. |
| Rotation interval | [Shadcn Space Select 01](https://shadcnspace.com/components/select) | Reuse the simple labeled control for interval presets plus Custom. |
| Shuffle | [Shadcn Space Switch 05 — With label](https://shadcnspace.com/components/switch) | Direct labeled toggle in Rotation, without description-heavy cards. |
| Import, rename and short settings forms | [Shadcn Space Dialog](https://shadcnspace.com/components/dialog) | Reuse the dialog primitive and restrained form layout; omit demo-specific payment/connector content. |
| Actionable sync/import error | [Space UI Status Badge](https://www.spaceui.one/components/status-badge) | Optional; use animated=false and show only relevant errors or detail-level states. Never restore a permanent success badge in Library. |

### Underline navigation: observed and pending

The live demo changed the selected tab when clicked, and its source was publicly viewable. Its code uses Motion, Lucide icons and the shared Tabs primitives. The inspected example did **not** include reduced-motion handling or tab-panel markup; its animated body mounts/unmounts. Restore proper panel relationships and preserve user state, or use route links with the same visual treatment for the three main destinations. These are integration fixes, not a new design exercise.

The live source dialog showed the Base UI install identifier @shadcn-space/tabs-05. The site's documentation offers a Radix variant through its library selector. Confirm the exact Radix registry identifier during setup rather than assuming the demo's default matches our stack.

### Gallery: observed and pending

The [grid API](https://www.spaceui.one/components/adaptive-density-grid) exposes custom item rendering, responsive column settings, control visibility, animation duration and stagger timing. The page states MIT licensing. Its demo's remote images did not load in this browser; a successful complete visual demo was therefore not established. The displayed example also imports next/image, which must be replaced with the app's Vite-compatible image rendering. This is a candidate pending a small local trial, not a validated production gallery.

The demo's content-visibility option is a rendering optimization, not proof of virtualization or suitability for thousands of full-resolution wallpapers.

### Image preview: observed and pending

The [Planes lightbox](https://useplanes.com/components/photo-lightbox) opened an image, showed explicit previous/next/close controls, and advanced to the next image in the live demo. Escape dismissal was not confirmed in this inspection; focus restoration, keyboard behavior and reduced motion still need verification in our app.

The page marks it Pro and advertised a $99 one-time Planes Pro purchase when inspected. No purchase or license ownership is assumed. Use the existing Radix-based preview while deciding whether this addition is worth paying for.

### Upload alternatives

[Planes File Dropzone](https://useplanes.com/components/file-dropzone) is another Pro option with documented thumbnail, cancellation, retry and progress support. It is better suited to a fuller sync/upload experience, but not needed to begin the component slice.

The [Shadcn Space upload page](https://shadcnspace.com/components/file-upload) marks variants 02–04 Pro; variant 01 is not Pro-marked. The examples must be connected to the existing app's import behavior. Restrict accepted files to supported wallpaper image types.

## How the supplied libraries compare

| Library | Decision for this app |
| --- | --- |
| Shadcn Space | Main source for standard app UI. The supplied [animated-block page](https://shadcnspace.com/animated-blocks) focuses on promotional page sections; its component catalog is the more useful part for this task. |
| Planes | Strongest specialized media candidate: Photo Lightbox. Free [Segmented Control](https://useplanes.com/components/segmented-control) is also usable, but avoid duplicating the main navigation library without a clear benefit. |
| Space UI | Useful grid and contextual status pieces. Use only the necessary components, not the site's full animated visual language. |
| Skecher UI | Selective reference rather than default foundation. [Apple Mail Tabs](https://skecher-ui.com/docs/apple-mail-tabs) hides inactive text labels and expands the active item; I would not use that behavior for our three main destinations. Its visible source includes keyboard handling and reduced-motion support, but the collapsing labels remain a product-fit issue. [Sliding Panel](https://skecher-ui.com/docs/sliding-panel) is a tab/content transition example, not a ready-made wallpaper settings sheet. |
| Componentry | Defer for this product's core screens. [Collection Surfer](https://componentry.dev/docs/components/collection-surfer) is a perspective scrolling scene with a very tall scroll area, not a practical library browser. [Hover Transition](https://componentry.dev/docs/components/hover-transition) could provide an optional reveal effect, but is unnecessary for the first slice. |

[Reaction Arc](https://useplanes.com/components/reaction-arc) is Pro and supports reaction or item-to-target feedback. It could inspire a later save animation, but the app first needs a straightforward Save/Saved action. Avoid turning the wallpaper app into a set of animation demonstrations.

## Licensing and verification

- [Shadcn Space licensing](https://shadcnspace.com/license) permits commercial use of its free materials and requires retaining its notice/license. Premium resources have separate terms.
- Space UI's inspected component pages state MIT licensing.
- Planes' shortlisted specialized media/import pieces are marked Pro.
- Componentry and Skecher component pages explicitly ask users to review relevant source licenses. No repository-wide commercial-use clearance was established for those two in this review.

Docs and selected live demos were inspected during research. This documentation commit records the component selections; component source is not yet vendored or installed. No purchase, production integration or full accessibility verification is implied.

## Next implementation slice

The existing frontend is in [apps/web](../apps/web).

For the next implementation slice, establish one shared component foundation and build a small **Library → open image → import → collection actions** slice from the selected source components, using real existing wallpaper assets. Compare that functioning slice before expanding to Discover and Rotation.

Preserve existing local persistence and scheduling behavior, the user's preference for minimal contextual information, and the distinction between a frontend preview and actual native desktop/cloud integration. The current design direction is also recorded in [frontend instructions](../apps/web/AGENTS.md). Follow the [motion guide](13-motion-and-ui-iteration.md) and verify imported components with our own data before adoption.
