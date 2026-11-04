# Walls collection prototype

First local frontend slice, 2026-11-04. React, Vite, Motion, Radix Dialog, Inter, and IndexedDB. Based on the approved compact Collections mockup.

Run `npm ci` then `npm run dev -- --host 0.0.0.0 --port 4173`. Build with `npm run build`; the retained packaging tests run with `npm run test:sites`.

Implemented: collection shelves/detail, all-wallpaper search, image preview/download, new collection, multi-image JPEG/PNG upload with per-file validation (25 MiB / 60 MP), folder rotation toggles, order/frequency presets, pause preference, local backup export, subtle motion and reduced-motion support. Local data is stored in the browser's `walls-local` IndexedDB database. Browser storage limits and clearing site data affect retention; export is a manual backup, not cloud sync. Upload originals remain local and are not sanitized or sent to a server in this prototype.

Google sign-in, server authorization, OCI uploads, scheduling, native wallpaper changes, and the admin console are not implemented. Rotation controls save preferences; they do not change the Mac desktop yet. Local settings cannot be used as production authorization.

The nine JPG wallpapers in `public/wallpapers` are generated illustrative content. They are not a scraped catalog or actual user photos. Assets were generated with the built-in Image Generation tool for the approved art direction: glider above clouds, aircraft wing at sunset, coastal flight, misty alpine valley, black-sand coast, layered dawn ridges, cream paper forms, limestone curves, and architectural shadows. See the root design brief for the reference image. Project code licensing remains pending the owner's selection; generated sample content does not grant rights in unrelated third-party images.

QA evidence and visual comparison are in `qa/` and `design-qa.md`. Keep the local preview open during iteration. Publishing and infrastructure work are separate delivery steps.

Settings now shares rotation controls with Edit and shows the next-change timing preview, countdown, timezone, collection inclusion, pause, and a daily-at time. Interval previews align to the next local whole hour or three-hour clock boundary; daily previews follow browser-local wall time. The persisted dailyTime field are frontend-only until the server/native contract is extended. Run `npm test` for six timing tests, including DST and month boundaries.
