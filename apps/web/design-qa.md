# Collections prototype QA — 2026-11-04

final result: passed

## Evidence

Source: ../../docs/design/collections-refinement.png (1488 × 1058 pixels). Render: qa/desktop.png, 1440 × 1024 CSS/pixels, device scale 1, default Collections state. Source normalized to 1440 × 1024 for the combined comparison in qa/comparison.jpg. Focused comparison in qa/header-detail.jpg stacks the same heading/action region from source and render. Mobile: qa/mobile.png, actual emulated 390 × 844, scale 1. Initial CLI resize did not override a prior viewport emulation; those preliminary captures were excluded from the final mobile assessment.

## Comparison and fixes

- Initial desktop comparison showed excess row/footer spacing. Reduced heading gap, shelf gaps/padding, image aspect height, and footer padding; recaptured the same default state. All three shelves remain visible with consistent aligned tracks.
- Added explicit dialog focus restoration after keyboard dismissal. Browser confirmed Escape closes the preview and focus returns to the image button.
- Added a favicon and field names after initial console inspection. Latest reloaded console has no errors.

## Required fidelity surfaces

- Typography: Inter variable sans, modest bold page heading, lighter 14–17 px controls and summary. Clear hierarchy and wrapping; actual font replaces the generated mock's approximate letterforms.
- Layout: compact top navigation, right-aligned primary action, three unboxed image shelves and inclusion controls. Equal three-column desktop tracks and two visible shelf previews on narrow mobile; full collection view exposes all images. No horizontal overflow at either tested viewport.
- Colors/tokens: near-white background, graphite text, muted grey supporting text, restrained forest green actions. Focus rings and native checkbox state remain visible.
- Imagery: all nine generated JPG assets load, have the intended photographic subjects, and use object-fit crops. Independently generated aircraft/coast/ridge/shadow compositions vary from the mock; subject, tone and visual role are preserved. No placeholder or CSS-drawn imagery.
- Copy/content: collection names, Open links, inclusion labels, summary and actions match the reference. Added a quiet local-preview disclosure and truthful integration limitations in settings/rotation dialog.

## Browser interaction checks

Created QA mornings, checked its empty state, uploaded a JPEG through the browser file-input change path, observed Added status, and reloaded to verify collection/image/preferences persisted in IndexedDB. Tested In order and three-hour cadence updates, All wallpapers navigation and name search (one matching Alpine result), image preview, Escape dismissal and focus restoration. QA collection removed and defaults restored after testing. Mobile fits at 390 px without horizontal overflow. Code includes per-file MIME/byte/pixel/decode checks, but malformed/oversized cases and actual OS reduced-motion changes were not exercised in this pass.

## Remaining polish and limits

P3: independently generated image crops and the navigation underline position differ slightly from the concept. The local preview disclosure is additional product copy. No actionable P0/P1/P2 issues remain in this frontend slice. This is not full accessibility certification or native macOS/Google/OCI verification. Cloud authentication, backend data ownership, scheduling and wallpaper application are future implementation work.

## Checklist

- [x] Reference/render full-view and readable header comparison
- [x] Desktop/mobile image loading and layout
- [x] Main collection/upload/preferences path and reload persistence
- [x] Error-free latest browser console
- [x] Build and retained packaging checks

## Settings timing refinement

User requested fuller Settings plus next scheduling/time controls. Settings now uses the same RotationControls component as Edit, with one next-change preview, countdown, local timezone, daily-at input, collection selection, and pause. Checked daily 18:30 updates next time to 6:30 PM; pause replaces countdown; reload restores daily time. At 390 × 844 dialog width is 358 px with no horizontal overflow and internal scrolling. Screenshots: qa/settings-desktop.png and qa/settings-mobile.png. No new browser console errors. Five schedule tests and four packaging tests pass; production build passes. Local timing does not apply wallpapers on macOS.
