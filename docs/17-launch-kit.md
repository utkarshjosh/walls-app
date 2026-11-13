# Launch kit: announcing the beta

Written 2026-11-12, when the app became a free download. This is the plan for telling people about it, with the words ready to paste. Nothing here has been posted. Every claim in the copy below is something the app does today; if a feature changes, change the copy.

## Before posting anywhere

A launch post sends a few hundred to a few thousand people to the download button in one day, and they write about what happens next. Fix these first, in this order.

| Item | Why it matters on launch day | State |
| --- | --- | --- |
| Deploy the landing page and the new server | The download button, sign-in and the admin portal have to be live | Done 2026-11-12 |
| Publish a build | Until then the page says "The first build is being prepared" | Done 2026-11-13: beta 0.1.0, Apple silicon only |
| Sign and notarise the app (Apple Developer Program, US$99 a year) | Without it every Mac refuses to open the app until its owner approves it in System Settings, and some managed Macs never allow it. On a launch thread this is the first comment and it crowds out everything else | Not done |
| An Intel or universal build | The only build today is for Apple silicon. Either make one (see [the desktop README](../apps/desktop/README.md#releasing-a-beta)) or say "Apple silicon only" in every post | Not done |
| Decide the name | "Still" is a working name. A rename after launch throws away every link and search result the launch earned, and the bundle identifier cannot change once people have installed it | Open |
| A contact address on the privacy page | People need a way to reach you; so do the directories below | Missing |
| A mail relay, if you want sign-ups on day one | Without it the app simply does not offer signing in. Launching without it is fine; you then learn from downloads and anonymous totals only | Not set up |
| A licence in the repository | The repository is public, but with no licence it is "source visible", not open source. Do not call it open source in a post until there is one | Missing |
| More wallpapers | Discover is the first thing people open. A thin catalog reads as an unfinished app | Owner's call |

## Where to post, and in what order

Start small, fix what the first people trip over, then go wider. Each place has its own rules about makers posting their own work: read them on the day, they change.

1. **People you know, and the early list.** Anyone on the "Early list" tab of the admin portal asked to be told. Write to them first, by hand. Their first-launch problems are the cheapest ones to hear about.
2. **Mac communities on Reddit** (r/macapps is the main one for new Mac apps). Post once, as the maker, with a screen recording. Stay in the thread for the day and answer everything.
3. **Hacker News, as a "Show HN".** Works when there is something technical to talk about, and here there is: live wallpapers as single sandboxed files that cannot reach the network, and one picture laid across displays of different sizes and densities. Post on a weekday morning, US time.
4. **Product Hunt.** Needs a day of your attention and a gallery of images. Worth doing once the app is notarised, not before.
5. **Short posts with a video** on X, Bluesky, Mastodon and Threads. The product is visual: a ten-second recording of one wallpaper stretching across two displays says more than any sentence.
6. **Directories that people search**: AlternativeTo (list it as an alternative to the wallpaper apps people already know), MacUpdate, and a Homebrew tap of your own. Homebrew's main cask list expects a notarised app.
7. **Writers.** A three-line email to the tips address of the Mac blogs and newsletters you read yourself, with the link, one image and the one thing that is different. Only after it is notarised.

## The one thing to say

Lead with what no built-in setting does: **one wallpaper across every display you own, set up by itself.** Everything else supports it.

| Length | Text |
| --- | --- |
| A line | Wallpapers for the whole desk: one picture across every display on your Mac. |
| Two lines | Still is a free wallpaper app for the Mac. It runs one picture across all your displays, changes your desktop on a schedule, and plays live scenes behind your icons. |
| A paragraph | Still is a free wallpaper app for the Mac, in beta. Connect a second display and it offers to run one wallpaper across both, following the arrangement you set in System Settings. It checks every picture against the displays you have, so nothing is ever stretched into a blur. It changes your desktop on the hour, from the collections you choose, and it plays live scenes behind your icons: small files that can draw and nothing else. No account is needed. |

Say these plainly, because people will ask: it is free while in beta; it needs macOS 12 or later; signing in is optional and the privacy page lists exactly what is recorded if you do; there is no Windows or Linux version.

## Ready to paste

### Show HN

Title:

```
Show HN: Still – one wallpaper across every display on your Mac (free beta)
```

First comment:

```
I made Still because I wanted one picture to run across my laptop and my monitor, and every way of doing it by hand broke the moment I moved a display.

What it does:
- Connect a second display and it offers to run one wallpaper across all of them, following the arrangement in System Settings. Each display gets its own piece at its own pixel size.
- It checks each wallpaper against the displays connected right now. A picture too small for a Retina display is skipped, not stretched.
- It changes the desktop on the hour (or every three hours, or at a time of day) from the collections you pick, from the menu bar, with the window closed.
- Live wallpapers are single HTML files drawn behind the desktop icons. Each carries a content policy that stops it loading or sending anything, and the app refuses a file that does not have it.

It is a Tauri app: a Rust core that talks to AppKit, and a React interface. The catalog is a small Go service with SQLite.

It is free and in beta. No account is needed; signing in is optional and the site says exactly what is recorded if you do. macOS 12 or later.

I would like to hear where it breaks, especially with unusual display arrangements.
```

### Reddit (r/macapps and similar)

Title:

```
I made a free wallpaper app that runs one picture across all your displays
```

Body:

```
Still is a wallpaper app for the Mac. I built it because I use a laptop next to a monitor and wanted one picture across both without lining it up by hand.

- One wallpaper across every display, following your arrangement in System Settings
- Or a different one on each, or the same on all
- Changes on a schedule from collections you choose, from the menu bar
- Skips pictures that are too small for your displays instead of blurring them
- Live scenes that play behind your desktop icons

Free while it is in beta, no account needed, macOS 12 or later.

Download: https://walls.utkarshjoshi.com

It is early. Tell me what breaks.
```

### Product Hunt

- Name: `Still`
- Tagline (60 characters at most): `One wallpaper across every display on your Mac`
- Description (260 at most): `Still is a free wallpaper app for the Mac. It runs one picture across all your displays, changes your desktop on a schedule from collections you choose, never shows a blurry picture, and plays live scenes behind your icons. No account needed.`
- Topics: Mac, Wallpapers, Productivity, Design Tools
- Gallery: `apps/site/public/media/og.jpg` first, then the three app screenshots in `apps/site/public/media/app/`, then a recording of the hero.

### Short posts

```
One wallpaper, across every display on your Mac. Still is free while it's in beta. https://walls.utkarshjoshi.com
```

```
Plug in a second display and your wallpaper just continues onto it. That's the whole pitch. Still, a free wallpaper app for the Mac: https://walls.utkarshjoshi.com
```

```
Live wallpapers for the Mac that are one small file each and cannot touch the network. They play behind your icons. Free beta: https://walls.utkarshjoshi.com
```

## Pictures and video

| What | Where it is | Still needed |
| --- | --- | --- |
| Link card (1200 wide) | `apps/site/public/media/og.jpg`, used automatically when the page is shared | Nothing |
| Three app screenshots | `apps/site/public/media/app/` | Retake after any interface change (`npm run capture` in `apps/site`) |
| The hero as a recording | Not made | Ten to fifteen seconds of the landing page's hero scrolling through its three states, or better, a real two-display desk |
| App icon | A placeholder | The real one, before any directory listing |

## Reading the result

The admin portal's Activity tab shows, by day: page visits, downloads, new users and active users. The useful number on launch day is downloads divided by visits; if it is low, the page or the first-open warning is the problem, not the reach. The Users tab shows what the people who signed in actually did, and "Saved by the most people" shows which wallpapers earned their place.

The visit count deliberately carries nothing about where a visitor came from, so the portal cannot tell Reddit from Hacker News. Post to one place at a time, a day apart, and read the bump.
