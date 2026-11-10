/**
 * The contract between the shared React UI and whatever hosts it.
 *
 * Two hosts implement it:
 *  - web.ts     — a browser tab. Nothing here can touch the real desktop, so
 *                 wallpaper changes are a labelled in-app preview.
 *  - desktop.ts — the Tauri shell. Each method maps to one Rust command in
 *                 apps/desktop/src-tauri (names are given beside each method).
 *
 * The UI never imports Tauri directly; it only talks to `Platform`.
 */

/** image: a still picture. video: a looping movie. scene: a self-contained animated HTML file. */
export type WallpaperKind = "image" | "video" | "scene";

export interface DisplayInfo {
  /** Stable while the display stays connected (CGDirectDisplayID as a string on macOS). */
  id: string;
  /** Human name, e.g. "Built-in Retina Display" or "Studio Display". */
  name: string;
  /**
   * Position and size in points within the global desktop.
   * Origin is the top-left of the primary display, y grows downward.
   * (Convert from AppKit's bottom-left origin on the native side.)
   */
  x: number;
  y: number;
  width: number;
  height: number;
  /** Pixels per point (2 on Retina). */
  scale: number;
  primary: boolean;
}

/** A file the host keeps for the library (imported or saved from Discover). */
export interface StoredFile {
  /** The name it was stored under, e.g. "a1b2c3.jpg". Flat namespace, no folders. */
  name: string;
  /** Absolute path on disk (desktop) or an opaque key (web). */
  path: string;
  /** A URL the UI can put in <img>/<video>/<iframe>. */
  url: string;
  bytes: number;
}

/** One thing the host can put on the desktop. */
export interface PlayItem {
  /** Wallpaper id, echoed back in RotationStatus so the UI knows what is showing. */
  id: string;
  kind: WallpaperKind;
  /** StoredFile.path of the full-resolution file (image, video, or scene .html). */
  path: string;
  /**
   * StoredFile.path of a still image. For video/scene wallpapers the host also
   * sets this as the ordinary desktop picture, so the lock screen and Mission
   * Control show a matching still. Absent for plain images.
   */
  posterPath?: string;
}

/**
 * How wallpapers map onto displays. Only meaningful with 2+ displays;
 * with one display every mode behaves like "same".
 *  - same: one wallpaper, repeated on every display.
 *  - span: one wallpaper stretched across all displays as a single canvas,
 *          following the real arrangement.
 *  - each: a different wallpaper per display.
 */
export type DisplayMode = "same" | "span" | "each";

/** Put something on the desktop right now. Command: `wallpaper_apply`. */
export interface ApplyRequest {
  mode: DisplayMode;
  /**
   * same/span: items[0] is used.
   * each: items[i] goes to the i-th display ordered left-to-right, then
   *       top-to-bottom; if there are fewer items than displays they repeat.
   */
  items: PlayItem[];
  /** With mode "same": change only this display and leave the others alone. */
  displayId?: string;
}

export type Schedule =
  /**
   * Change every `minutes`. Changes land on local clock boundaries counted
   * from local midnight (60 → every :00; 180 → 00:00, 03:00, …; 15 → :00, :15, …),
   * never offset by when the app was opened. The count restarts at midnight.
   */
  | { type: "interval"; minutes: number }
  /** Change once a day at local `time`, "HH:MM" 24-hour. */
  | { type: "daily"; time: string };

/** Everything the host needs to rotate wallpapers with the window closed. Command: `rotation_sync`. */
export interface RotationPlan {
  /** Every wallpaper in the collections the person chose, de-duplicated, in library order. */
  items: PlayItem[];
  shuffle: boolean;
  schedule: Schedule;
  paused: boolean;
  displayMode: DisplayMode;
  /** Freeze video/scene wallpapers while on battery power. */
  pauseLiveOnBattery: boolean;
}

export interface RotationStatus {
  paused: boolean;
  /**
   * What is on the desktop now. One entry with displayId null when every
   * display shows the same thing (modes same/span); one entry per display for "each".
   * Empty when the app has not set a wallpaper yet.
   */
  current: { displayId: string | null; itemId: string }[];
  /** Epoch milliseconds of the next automatic change, or null when paused/empty. */
  nextChangeAt: number | null;
  /** Whether "previous" has history to go back to. */
  canGoPrevious: boolean;
}

export type RotationCommand = "next" | "previous" | "pause" | "resume";

export interface Platform {
  /** "web" cannot change the real desktop; "desktop" can. */
  readonly kind: "web" | "desktop";
  /** False in a browser tab: the UI then labels wallpaper actions as a preview. */
  readonly canSetWallpaper: boolean;

  /** Command: `displays_list`. */
  displays(): Promise<DisplayInfo[]>;
  /** Event: `displays-changed`, payload DisplayInfo[]. Returns an unsubscribe function. */
  onDisplaysChanged(listener: (displays: DisplayInfo[]) => void): () => void;

  /**
   * Keep a file in the library. Command: `library_store` — the bytes are the raw
   * request body, the name travels URL-encoded in the `x-file-name` header.
   * Storing the same name again replaces the file.
   */
  storeFile(name: string, data: Blob): Promise<StoredFile>;
  /** Fetch a remote file straight into the library. Command: `library_download` { name, url }. */
  downloadFile(name: string, url: string): Promise<StoredFile>;
  /** Command: `library_remove` { names }. Missing files are ignored. */
  removeFiles(names: string[]): Promise<void>;
  /** Turn a stored name back into a usable URL after a restart. Command: `library_resolve` { names }. */
  resolveFiles(names: string[]): Promise<Record<string, StoredFile>>;

  /** Command: `wallpaper_apply` { request }. */
  apply(request: ApplyRequest): Promise<RotationStatus>;
  /** Command: `rotation_sync` { plan }. Safe to call often; does not change the wallpaper by itself. */
  syncRotation(plan: RotationPlan): Promise<RotationStatus>;
  /** Command: `rotation_command` { command }. */
  rotationCommand(command: RotationCommand): Promise<RotationStatus>;
  /** Command: `rotation_status`. */
  rotationStatus(): Promise<RotationStatus>;
  /** Event: `rotation-changed`, payload RotationStatus. Returns an unsubscribe function. */
  onRotationChanged(listener: (status: RotationStatus) => void): () => void;
}

/**
 * Live wallpaper windows (video and scene) receive their geometry in the URL
 * fragment so one wallpaper can span several displays seamlessly:
 *
 *   #cw=<canvas width>&ch=<canvas height>&vx=<this window's x>&vy=<this window's y>&paused=0|1
 *
 * All values are points. The canvas is the whole surface the wallpaper covers
 * (one display for "same"/"each", the union of all displays for "span"); vx/vy
 * is this window's top-left corner inside that canvas. Scenes derive animation
 * time from the wall clock so separate windows stay in step.
 *
 * Videos are played by the bundled page `live.html`, which additionally takes
 *   &src=<url-encoded video url>
 * Scenes are loaded directly from their stored .html file.
 */
export interface LiveViewport {
  cw: number;
  ch: number;
  vx: number;
  vy: number;
  paused: boolean;
}
