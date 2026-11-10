# Motion and ongoing UI iteration

Decision, 2026-11-03: keep the collection experience simple and clean, with slight animation. Continue improving the UI through small, understandable changes. This document describes motion intent. The first local React prototype now uses Motion for dialogs and quiet status feedback.

## Component reuse update — 2026-11-08

Use the [component foundation](14-component-foundation.md) for current source selections and product direction. Imported components must follow this motion policy: library defaults such as animated grid reflow, spring overshoot, collapsing navigation labels and perpetual status pulses are not automatically accepted. Verify keyboard behavior, focus restoration and Reduced Motion in the integrated app. Preserve the current scheduling rules and local data while restyling. The earlier product-delivery order remains separate from the expanded Library / Discover / Rotation design target.

## Owner update — 2026-11-10

After using the first build the owner asked for more "motion graphics, feedback" and smoother Discover tiles. This widens the earlier, very quiet default without changing its limits.

Now expected: a cross-fade when the wallpaper on a display changes; a sliding highlight on segmented choices; a slight dip when a control is pressed; state marks (saved, selected) that pop in rather than appear; tile pictures that ease in a few percent under the pointer while the tile itself stays put.

Still not allowed: animating gallery layout, spring overshoot, staggered entrances, anything that moves without the person having done something, and motion that ignores Reduced Motion. Do not put a blur behind controls that sit on tiles; it makes long grids stutter while scrolling in the Mac app's web view.

## Library and scope

Use the free **Motion for React** package (`motion`, imported through `motion/react`) for transitions that need React lifecycle coordination. Simple hover/focus color changes can use CSS. Use one motion library, and pin its version when the frontend is created. No premium animation package is required. [Motion React documentation](https://motion.dev/docs/react), [official documentation index](https://motion.dev/llms.txt)

| Interaction | Proposed treatment |
| --- | --- |
| Collection/image becomes ready | Brief opacity fade after decoding; reserve dimensions to prevent content jumps |
| Rotation settings sheet | Fade with at most 6 px translation, around 180–220 ms; focus moves correctly and returns on close |
| Button/toggle feedback | Immediate state change; subtle color/opacity transition around 100–140 ms |
| Save/upload completion | Quiet inline confirmation; no confetti or repeated motion |
| Collection navigation | Short content fade, around 160–180 ms; controls remain usable throughout |

These timings are design defaults to tune in the running prototype, not library defaults or measured performance. Prefer opacity and transform. Avoid spring overshoot, staggered gallery entrances, parallax, scroll hijacking, perpetual animations, and animated grid rearrangements. Do not delay a successful action until an animation completes. Animation concerns the collection UI; it makes no promise of cross-fades between macOS desktop wallpapers.

Use `MotionConfig` with `reducedMotion="user"` and `useReducedMotion` where needed. When Reduced Motion is enabled, remove spatial movement and use immediate state changes or a short opacity transition. Verify the setting at runtime, including CSS transitions; do not assume one library setting covers every animation. [Motion configuration](https://motion.dev/docs/react-motion-config), [Reduced Motion hook](https://motion.dev/docs/react-use-reduced-motion)

## Keep future edits straightforward

Keep colors, typography, spacing, radii, motion durations, and easing in a small shared token file when implementation starts. Prefer a few named components such as CollectionShelf, WallpaperThumbnail, RotationSettingsSheet, and UploadStatus. Use the same components in desktop and browser hosts; inject platform behavior rather than duplicating screens.

Keep animation recipes beside shared UI components or in one small motion module. Avoid scattering one-off timings across pages. Keep sample data, real API transport, and native commands separate. Preserve loading, empty, error, offline, keyboard, and reduced-motion behavior when restyling.

For LLM-assisted work, start with the design brief, this guide, and the current implementation before making changes. Motion offers an [LLM documentation index](https://motion.dev/llms.txt); read only relevant sections. No new global skill, MCP server, paid service, or model runtime is needed for this plan. Product discovery and prompt-based curation remain later phases.

Update the design brief when product behavior or visual conventions change. Validate actual interactions in a browser, and measure idle desktop behavior separately. Do not mistake a generated concept image or static screenshot for a tested implementation.
