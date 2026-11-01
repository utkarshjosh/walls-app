# Product brief

Date: 2026-11-01. Working name: Wallpaper Server. Owner: Utkarsh Joshi. Revised for the collection-first, carefree product direction.

## Idea

Collect wallpapers you like and let your desktop take care of itself. Browse images, save them into folders, and connect one or more folders to your Mac. Rotation is generated automatically and quietly follows changes to those folders.

The desktop app is a place to enjoy and organize images, using a collection interface shared with the optional browser app. The owner has a separate admin web surface. Routine use requires no timetable, queue inspection, playlist maintenance, or operational monitoring. Reliable prefetching and scheduling happen behind the experience.

## Principles

- A useful setup exists as soon as onboarding finishes.
- Default to hourly shuffle, automatic daily downloads, and automatic caching.
- Collections are the user's mental model; a separate playlist is unnecessary.
- Saving into a connected folder changes future rotation after the next successful sync.
- Healthy operation stays quiet. Ask for attention only when the user can usefully act.
- Keep core controls small. Weekly scheduling and queue editors are outside MVP, rather than hidden in a large advanced-settings panel.
- No advertising, paid placement, tracking pixels, or sale of personal preferences.
- Personal uploads remain private unless explicitly submitted for later public discovery.

## First use

After installing/opening the Mac app and signing in:

1. Pick a few interests or starter collections. Alternatively use a general starter set or upload personal images.
2. Connect this Mac in the same app.
3. Wallpapers start automatically from those folders, shuffled hourly.

Browser-first setup remains supported, but routine management never requires a second website.

Interest tags map to a small, manually curated set of permitted starter images. This is deterministic onboarding, not AI or a live discovery service. Starter selections create private collection memberships referencing approved assets; later template edits do not silently change personal folders. Rights withdrawals still propagate.

No required cadence, sync time, timezone, or future-image choices. Defaults and selected folders can be changed later.

## Everyday use

Browse collections, upload images, and save/remove favorites. A short summary reads “Aviation and Landscapes, shuffled every hour.”

Regular configuration is limited to **which folders**, **Shuffle or In order**, and **how often** (every hour, every three hours, every day). “In order” means sequential traversal, not real-time synchronization between devices. Saving into a connected collection requires no second “add to playlist” action.

The Mac menu offers Next, Pause/Resume, and Open collection. Connection help and Sync now are secondary troubleshooting actions. Temporary network/server failures retain a working desktop.

## Vocabulary

**Collection/folder:** saved wallpapers grouped by category; an image may belong to several. **Connected collections:** the folders currently feeding rotation. **Rotation preferences:** selected folders, order mode, and frequency. **Schedule/manifest:** internal generated playback data, not a user-authored calendar. **Companion:** the local Mac application, now including the full collection interface. **Starter collection:** reviewed sample content for onboarding.

Playlist remains a useful analogy and a possible future curation output. MVP has no separate playlist entity/editor.

## Scope

| Phase | Included | Outside this phase |
| --- | --- | --- |
| Core MVP | Interests/starter folders, private uploads, flat collections, live connection of one/many folders, shuffle/sequential, frequency presets, daily prefetch, offline playback, Next/Pause, export/deletion | Playlist editor, weekly rules, timetable/queue preview, per-image durations, folder weights, pin timers, per-display settings, public discovery, behavioral learning, prompts |
| Discovery later | Pinterest-style browse/save, approved external content, explicit user submissions, attribution, moderation, interest filtering | Required AI or learning dependency |
| Intelligence later | Two independent opt-in additions: taste learning through similarity/KNN/clustering, and natural-language collection/rotation curation | Mandatory chat UI, automatic publication, unreviewed destructive actions |
| Platform extensions | Android and a separately validated iOS workflow | Identical automation guarantees across operating systems |

Discovery and intelligence are optional follow-on phases. The core product must remain useful without either.

## Success criteria

- Automatically configured rotation in three short onboarding steps without schedule settings.
- Preference selection under two minutes in a usability trial, excluding sign-in, installation, pairing, and transfers.
- A user can describe the setup as “these folders, shuffled, every hour.”
- Connected collection edits flow into rotation without manual republishing.
- Daily sync and 72-hour offline playback work on real hardware; cached repeats may sustain a seven-day plan.
- Thirty-day personal dogfood needs no routine schedule or cache management.
- No duplicate downloads of already verified variants.
- Public discovery requires rights review and moderation.

These are proposed targets. Free-to-use and ad-free does not imply zero hosting, storage, or distribution costs.
