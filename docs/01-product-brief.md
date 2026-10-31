# Product brief

Date: 2026-10-31. Working name: Wallpaper Server. Owner: Utkarsh Joshi.

## Idea

Treat wallpapers like a music library: discover something beautiful, save it, organize it into categories, assemble a playlist, and decide when it plays. The desktop companion quietly downloads ahead of time and keeps working without an internet connection.

The initial user is the owner with a Mac desktop. The later audience is a small community that shares a discovery catalog while keeping personal collections and schedules private. The web app manages the library; the Mac app applies images in the user's login session.

## Product principles

- No advertising, sponsored ranking, tracking pixels, or sale of personal preferences.
- Users control their files, playlists, timing, and storage usage; support export.
- A downloaded wallpaper remains useful without the server being available.
- Explain what will play next and why; interests begin as explicit choices.
- Discovery sources must permit the operations we perform, including thumbnails, copying, redistribution, and offline use where applicable.
- Start with still images and a small operational footprint.

## Vocabulary

**Wallpaper:** an image plus provenance, dimensions, rights information, and usable variants. **Library item:** a user's saved reference to a wallpaper. **Collection:** a category-like folder, such as Aviation or Landscapes; an item may belong to several. **Playlist:** a deliberately ordered set of library items, optionally shuffled. **Schedule:** rules choosing a playlist and rotation frequency. **Manifest:** an immutable, versioned list of future wallpaper changes for one device. **Companion:** the local macOS menu bar application, informally called the plugin.

Collections and playlists are separate: categorizing a photograph does not automatically put it into a running queue. MVP offers “Create playlist from collection” as a snapshot; live collection-backed playlists come later.

## Main journeys

1. Import a folder of existing images into the browser, put them in Aviation and Landscapes, create a playlist, pair the Mac, and see the first image appear.
2. Choose a 22:00 sync and a 60-minute wallpaper interval. The Mac downloads upcoming images that night; it rotates the next day with Wi-Fi off.
3. Browse a shared catalog using aviation, landscape, motivation, and shows interests. Save a licensed image to a private collection without making the collection public.
4. Use menu bar controls to pause, skip, pin the current image temporarily, sync now, and inspect cache status.
5. Discover a broken or inappropriate image, hide it personally or report it to the catalog moderator.

Shows and fan art are valid personal categories; they are not evidence of permission to redistribute TV/film imagery. Uploaded private files are never automatically published to discovery.

## Scope by release

| Release | Included | Excluded |
| --- | --- | --- |
| Personal MVP | Invite-only owner account, uploads, tags, flat collections, ordered/shuffled playlists, weekly time windows, daily prefetch, one Mac, all attached displays using the same image, pause/skip/pin, backup/export | Public registration, community uploads, arbitrary web scraping, video/live wallpaper, dedicated lock-screen control |
| Shared beta | Multiple accounts, common read-only discovery, personal interests, licensed source adapters, provenance, report/takedown workflow, quotas | Social graph, payments, ML recommendations, public sharing of private libraries |
| Later | Per-display assignments, live playlists, Android companion, mobile crops, optional iOS export/Shortcuts workflow | An assumption that mobile background automation works identically on every OS |

## Success criteria

- First upload to first desktop wallpaper within ten minutes for a new user, excluding download/install time.
- Daily sync and 72 hours of offline rotation demonstrated on a real Mac; seven-day target when the cache budget permits.
- Users can preview the next 24 hours and understand the active collection/playlist and next change.
- No duplicate binary downloads when the manifest references an already verified cached variant.
- Thirty-day personal dogfood period without a blank desktop caused by a failed sync or a corrupt image.
- Shared beta launches only when every public entry has a source, rights decision, and moderation status.

These are release targets, not measured results. Hosting can fit existing capacity, but “free to use, no ads” does not imply zero storage, bandwidth, domain, or signing costs.
