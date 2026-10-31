# Existing products and build decision

Research date: **31 October 2026**. Sources below are official product pages, documentation, developer-maintained repositories, and store listings. This is a bounded comparison, not an exhaustive market survey or hands-on audit. Free, open source, and ad-free are different claims.

## Findings

| Product | Platform and price/ad evidence | Useful overlap | Gap against this brief |
| --- | --- | --- | --- |
| macOS Wallpaper settings | Included with macOS; no separate app purchase or ad service in this workflow | Local folders/photo sets and shuffling | No self-hosted discovery, browser curation, or explicit server-to-device manifest protocol. [Apple guide](https://support.apple.com/en-gb/guide/mac-help/mchlp3013/26/mac/26) |
| Plash | Mac; developer explicitly lists it as free without ads | Uses a website as desktop wallpaper; automation possibilities | A web wallpaper surface, not a complete offline image library/server. A displayed third-party website can still carry its own ads. [Product](https://sindresorhus.com/plash.html), [free-app policy](https://sindresorhus.com/apps/free) |
| Equinox | Mac; official site says free and open source. An ad-free claim appears in a quoted user review, so publisher-level ad policy is not independently confirmed here | Creates time-, appearance-, or solar-driven dynamic wallpapers | Creation tool, not shared discovery and server sync. [Official site](https://equinoxmac.com/) |
| Irvue | Mac; currently listed as free with in-app subscriptions. Store listing alone does not establish ad-free behavior or which features remain free | Scheduled rotation, channels, multiple displays, Unsplash collections | Hosted source integration rather than your own storage and backend; not a confirmed wholly free match. [Mac App Store](https://apps.apple.com/us/app/irvue-desktop-wallpapers/id1039633667?mt=12) |
| Variety | Linux; open-source repository; no explicit no-ad promise verified in this review | Downloads, local images, timed rotation, curation | Linux client; no matching macOS/server/web system. Repository says maintenance mode. [Project README](https://github.com/varietywalls/variety) |
| Paperize | Android; free/open-source distribution, described by project as fully offline; no separate ad-policy audit | Local albums and automatic wallpaper changes; a useful mobile reference | Android local workflow, not the desired Mac/server workflow. [Project](https://github.com/Anthonyy232/Paperize), [F-Droid](https://f-droid.org/packages/com.anthonyla.paperize/) |
| Driftwood | Mac; official site says free and open source; no explicit no-ad policy verified here | Local playlists, durations, shuffle, display assignments | Focuses on animated/video scenes; the reviewed page does not document the proposed self-hosted discovery and daily image-prefetch backend. [Official site](https://driftwood.iamevan.dev/) |

**Best immediate zero-build option:** collect files into a local folder and use macOS shuffle. **Explicitly free/ad-free app to try:** Plash, when a website-backed desktop is desirable. Neither satisfies the complete product brief. Equinox is worth evaluating for dynamic wallpaper creation.

The product gap is the combination of private curation, playlist scheduling, controlled daily prefetch, offline native playback, owned storage, and a common discovery catalog. Build that combination; do not spend the first release reproducing dynamic/video rendering.

## Discovery changes the feasibility calculation

| Source | Evidence | Proposed treatment |
| --- | --- | --- |
| User-owned or explicitly permitted images | Rights supplied by uploader; keep a record | Primary MVP content; private by default |
| Wikimedia Commons | Reuse rules vary by file and require checking credit/license/derivative conditions. [Commons guidance](https://commons.wikimedia.org/wiki/Commons:REUSE) | First candidate adapter; initially allow reviewed CC0/public-domain material, then selected CC BY material after attribution review |
| Unsplash API | Official guidance explicitly identifies wallpaper apps and unofficial clients as problematic replication. [Unsplash guidance](https://help.unsplash.com/en/articles/2511257-guideline-replicating-unsplash) | Disabled by default. Seek explicit approval for this exact workflow before integration; existing third-party apps do not establish our permission |
| Pexels API | Official help says standalone wallpaper/gallery apps are not permitted, including personal/educational examples. [Pexels guidance](https://help.pexels.com/hc/en-us/articles/4405588861721-Can-I-use-the-API-as-a-wallpaper-app) | Disabled by default; do not assume a free key authorizes this product |
| Wallhaven | Official API page could not be retrieved in this research session (403). Unrelated lookalike domains were excluded | Candidate only; verify terms, API limits, image rights, and redistribution before enabling. [Official API entry](https://wallhaven.cc/help/api) |
| General web, image search, Reddit, Pinterest, fan-art sites | Search visibility and downloadable files do not establish copying or publishing rights | No unrestricted crawler. Add only explicitly approved adapters; links/bookmarks are not automatically downloadable catalog entries |

This is a product/engineering source policy, not a determination that every image on an allowed site is legally reusable. Record evidence per asset and support withdrawal. Implement the adapter interface early, but do not make the first release depend on external photo APIs.

## Research limitations

No apps were installed or tested. Repository descriptions were reviewed through public web indexing; the optional gh-axi CLI inspection was unavailable in this environment. Pricing and service terms can change; recheck at implementation and before public release. Unverified advertising behavior is explicitly labeled above rather than inferred from open-source status.
