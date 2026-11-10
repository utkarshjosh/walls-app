# Desktop Collections design history and current direction

## Current direction — 2026-11-08

The owner approved a **component-first approach**: reuse existing library components and review a small functioning Library slice, rather than continuing whole-screen image ideation. See the [component foundation](14-component-foundation.md) for the authoritative current design decisions, exact source links, selection statuses, integration caveats and rejected patterns.

The broader product has Library, Discover and Rotation jobs. Keep routine status and rotation controls out of Library, avoid duplicated navigation, and reveal detail when it helps the current task. The current palette exploration is neutral dark with a nonblue accent; no final accent or generated image has been selected. The earlier light/green direction and mandatory three-concept workflow below describe the prior prototype and are retained as history, not current implementation requirements. Existing local behavior and backend integration limits still apply.

## Earlier prototype brief — 2026-11-01 to 2026-11-04

Brief established 2026-11-01; direction updated 2026-11-03. User-selected tone: **calm, light, image-first**, with simple clean layouts and subtle motion. The first local React prototype is implemented in apps/web; the native Mac app remains planned. Working product label: Wallpaper Server, with “Walls” used only as a short mockup label until branding is chosen.

## Experience

The person using this app wants a pleasing desktop without managing future images. The main screen should invite browsing their collections. It should not feel like an operations dashboard or an automation editor. Open collection from the menu bar returns to this same interface inside the Mac app.

The first design target is the desktop Collections screen at 1440 × 1024. The app's primary navigation needs only Collections and All wallpapers, with Settings secondary. Discovery, AI, moderation, storage controls, device fleets, and future queues do not appear in the consumer MVP navigation. Admin is a separate browser surface.

One obvious primary action: **Add wallpapers**. New collection can be a quiet adjacent action or part of the upload flow. Collection covers carry the visual emphasis, with legible names and a clear “Use for wallpapers” state. Clicking a cover opens that collection; changing its rotation membership must be a distinct, labelled control.

A short sentence describes the current setup, such as “Aviation and Landscapes, shuffled every hour.” Its Edit action opens a small sheet with only selected folders, order, and frequency. Avoid duplicating this editor on the main page. No next-wallpaper timeline, schedule grid, metrics, streaks, or artificial engagement signals.

## Visual constraints

Light neutral surfaces, near-black text, generous spacing, and one restrained accent for actions/selection. Wallpaper photography supplies most color. Use a familiar sans/system family, 14–16 px body copy, clear modest headings, and no decorative all-caps labels. Actual implementation must verify contrast, focus visibility, reduced motion, and keyboard use.

Prefer a compact top navigation over a permanent global sidebar for two destinations. Keep collection names, controls, and images together; avoid the wide navigation rail and oversized feature image of the rejected third concept. Group through spacing and alignment, then dividers. Avoid nested cards, decorative shadows, fake OS chrome, and a landing-page hero inside the working app.

Use sample wallpaper subjects such as aviation, landscapes, ocean, and abstract forms, labelled **Sample collection imagery** in visual concepts. Generated concept imagery is illustrative; it is not a licensed launch catalog or actual user content. No fabricated user counts, customer claims, testimonials, source attribution, or deployment status.

## Design workflow and references

The user supplied [Hallmark](https://github.com/nutlope/hallmark) as design guidance, not as a screenshot to clone. Its [skill](https://raw.githubusercontent.com/Nutlope/hallmark/main/skills/hallmark/SKILL.md) and [app-screen guidance](https://raw.githubusercontent.com/Nutlope/hallmark/main/skills/hallmark/references/flow.md) were reviewed. Apply the app branch: restrained controls, explicit states, familiar navigation, typography and spacing. Marketing-page structure is not appropriate for this screen. No global skill installation is claimed.

Explore three independent image concepts before selecting an implementation target. Vary layout and hierarchy within the chosen tone: a collection-cover gallery, horizontal collection shelves, and a collection-focused album. These are alternative presentations of the same minimal scope. Resolve numeric choices against the order images are actually displayed in the chat. Carry the chosen reference into implementation; do not combine unchosen directions silently.

The Product Design workflow is being used for image ideation. No Hallmark code lint, browser accessibility check, or interactive test can pass until a frontend exists. A future landing page can use Hallmark's marketing branch after product behavior and authentic screenshots exist.

## Feedback and refined direction

The owner likes the second and first displayed concepts; the third feels too spread out. Retain the second's collection shelves and the first's compact header/actions and restrained green accent. The [refined mockup](design/collections-refinement.png) places each collection's name and inclusion control immediately above its three image previews, removing the left label column and keeping a consistent content width. There is no global sidebar or large featured-image area.

On 2026-11-03 the owner emphasized simplicity, cleanliness, slight animation, and ongoing iteration. Carry this combined mockup forward as the working implementation reference; it is not a frozen pixel-level design. It was generated with the built-in Image Generation tool using both liked concept images as references. Its [generation prompt](design/collections-refinement.prompt.md) records the direction. Sample imagery remains illustrative. The image is a visual reference; implementation verification is recorded separately in apps/web/design-qa.md.

Use Motion for React for restrained interaction transitions, with shared tokens and reusable components so human and LLM-assisted edits remain consistent. Follow the [motion and iteration guide](13-motion-and-ui-iteration.md). This development workflow does not bring the deferred AI product features into MVP.

## Required implementation states

| State | Expected behavior |
| --- | --- |
| First use | Pick interests/starter folders or own images; defaults create rotation without a schedule editor |
| Full | Browse covers/images; upload; use one or more folders; open small rotation sheet |
| Empty library | One clear action to add wallpapers, with starter selection as the alternative |
| Empty connected folder | Explain it needs images; retain desktop and use other selected folders |
| Loading | Skeletons shaped like covers; retain stable navigation and avoid layout jumps |
| Offline | Cached content and playback remain available; edits need a connection; no alarming full-screen takeover |
| Upload error | Per-file explanation and retry; successful files remain saved |
| Capacity reached | Explain that uploads are paused; preserve browsing and playback; deletion/settings provide recovery |
| Persistent connection failure | One actionable reconnect prompt; diagnostic details on request |

The first implemented UI slice is a responsive React prototype based on the combined reference, with sample data and working navigation, upload affordance, folder selection, rotation sheet, and subtle motion. Clearly distinguish simulated local interactions from real server/native integration. Verify the chosen screen plus empty/error/loading states in a browser before connecting the backend.

Settings refinement: the owner requested next scheduling and timing controls. The prototype now includes a single next-change preview/countdown and a daily-at time, alongside shared folder/order/frequency controls. This daily time choice updates the earlier default-only plan; there is still no future queue or calendar editor. Server/native timing contracts remain future integration work.

Modal scope correction: Settings and Edit show a compact selected-collection count plus Manage collections navigation. Folder membership is managed on the main Collections screen through its existing wallpaper toggles; no ever-expanding collection list belongs inside a modal.
