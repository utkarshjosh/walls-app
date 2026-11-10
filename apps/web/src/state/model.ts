import type { DisplayMode, WallpaperKind } from "@/platform/types";
import type { IntervalPreset } from "@/lib/schedule";

/** Where a picture's bytes are: a file the host keeps, or a URL (bundled sample / remote). */
export type Ref = { file: string } | { url: string };

export interface Wallpaper {
  id: string;
  title: string;
  kind: WallpaperKind;
  /** Pixels of the original; 0 when unknown (older local data). */
  width: number;
  height: number;
  origin: "sample" | "import" | "discover";
  full: Ref;
  thumb: Ref;
  /** Still frame for video and scene wallpapers. */
  poster?: Ref;
  /** Placeholder colour shown until the picture has loaded. */
  color?: string;
  author?: string;
  license?: string;
  sourceUrl?: string | null;
  /** Catalog tags. "clock" marks a scene that tells the time. */
  tags?: string[];
  /**
   * A scene drawn whole in every window rather than as one picture across
   * displays. Undefined on scenes saved before this was recorded; those are
   * looked up in the catalog once.
   */
  perDisplay?: boolean;
  /** Catalog id when saved from Discover; lets Discover show "Saved". */
  remoteId?: string;
  addedAt: number;
}

export interface Collection {
  id: string;
  name: string;
  inRotation: boolean;
  wallpaperIds: string[];
  /** Catalog pack this collection was saved from, if any. */
  packId?: string;
}

export interface Settings {
  shuffle: boolean;
  interval: IntervalPreset;
  customMinutes: number;
  dailyTime: string;
  paused: boolean;
  /** Remembered even while only one display is attached. */
  displayMode: DisplayMode;
  pauseLiveOnBattery: boolean;
}

export interface LibraryState {
  version: 2;
  wallpapers: Record<string, Wallpaper>;
  collections: Collection[];
  settings: Settings;
  /** One-time hints the person has already seen. */
  hints: { multiDisplay?: boolean };
  /** Which revision of the starter wallpapers this library has been offered. */
  starterSet?: number;
}

export const DEFAULT_SETTINGS: Settings = {
  shuffle: true,
  interval: "hour",
  customMinutes: 45,
  dailyTime: "07:00",
  // A fresh install never touches the desktop until the person starts rotation.
  paused: true,
  displayMode: "same",
  pauseLiveOnBattery: true,
};

/** Name of the collection that plain "Save" in Discover adds to. */
export const SAVED_COLLECTION = "Saved";

/**
 * Starter wallpapers, revision 3: thirteen stills rendered from the live scenes
 * (silk, glass, light trails, a planet's edge, a mountain lake) at 5120 x 3200,
 * with one 7680 x 2400 panorama per collection for stretching across two displays.
 * Revision 2 was twelve procedurally drawn ridges, gradients and dunes, which the
 * owner found "clean but very very basic"; revision 1 was nine small samples.
 * Earlier revisions are no longer offered to new installs but stay in libraries
 * that already have them, and those libraries gain the new collections.
 * The ids are the catalog's, so Discover shows these as already saved.
 */
export const STARTER_SET = 3;

const STARTERS: {
  id: string;
  name: string;
  items: [catalogId: string, slug: string, title: string, color: string, panorama?: true][];
}[] = [
  {
    id: "starter-silk",
    name: "Silk",
    items: [
      ["fcf40ca99748", "ink-silk", "Ink Silk", "#271f55"],
      ["f7d5984cf4bc", "black-satin", "Black Satin", "#171819"],
      ["5a4f17f170a5", "pearl-silk", "Pearl Silk", "#b8a49d"],
      ["317d5172c3ab", "garnet-drape", "Garnet Drape", "#531818", true],
    ],
  },
  {
    id: "starter-glass",
    name: "Glass and light",
    items: [
      ["e70f52435e95", "night-glass", "Night Glass", "#211b44"],
      ["1c294b47de40", "paper-glass", "Paper Glass", "#dad6c9"],
      ["cff97d7e6020", "black-glass", "Black Glass", "#151416"],
      ["98e43e5a7ad6", "light-trails", "Light Trails", "#26222c"],
      ["35a8589e7095", "glass-ribbon", "Glass Ribbon", "#1d1638", true],
    ],
  },
  {
    id: "starter-far",
    name: "Far places",
    items: [
      ["4772fa6e1517", "orbital-dawn", "Orbital Dawn", "#14182e"],
      ["68d2b14275d7", "red-limb", "Red Limb", "#2f1614"],
      ["b316c36e1cd2", "alpenglow", "Alpenglow", "#5a3f49"],
      ["b1db5dca5150", "long-limb", "Long Limb", "#0e1226", true],
    ],
  },
];

function starterWallpaper(
  catalogId: string,
  slug: string,
  title: string,
  color: string,
  panorama?: true,
): Wallpaper {
  return {
    // Same id a save from Discover would produce, so the two never duplicate.
    id: `d${catalogId}`,
    title,
    kind: "image",
    width: panorama ? 7680 : 5120,
    height: panorama ? 2400 : 3200,
    origin: "discover",
    full: { url: `/wallpapers/starter-${slug}.jpg` },
    thumb: { url: `/wallpapers/starter-${slug}-thumb.jpg` },
    color,
    author: "Still Originals",
    license: "Still Original",
    remoteId: catalogId,
    addedAt: 0,
  };
}

/** Add the starter collections to a library, without duplicating anything it already has. */
function withStarters(state: LibraryState): LibraryState {
  const wallpapers = { ...state.wallpapers };
  const collections = state.collections.map((collection) => ({ ...collection }));
  for (const starter of STARTERS) {
    const ids: string[] = [];
    for (const item of starter.items) {
      const wallpaper = starterWallpaper(...item);
      wallpapers[wallpaper.id] ??= wallpaper;
      ids.push(wallpaper.id);
    }
    const existing = collections.find(
      (collection) => collection.name.trim().toLowerCase() === starter.name.toLowerCase(),
    );
    if (existing) {
      existing.wallpaperIds = [...new Set([...existing.wallpaperIds, ...ids])];
    } else {
      collections.push({ id: starter.id, name: starter.name, inRotation: true, wallpaperIds: ids });
    }
  }
  return { ...state, wallpapers, collections, starterSet: STARTER_SET };
}

const SAMPLE_SIZE = { width: 1672, height: 941 };
/** Four of the bundled samples are 3:2 rather than 16:9. */
const SAMPLE_SIZES: Record<string, { width: number; height: number }> = {
  coast: { width: 1536, height: 1024 },
  "coastal-flight": { width: 1536, height: 1024 },
  ridges: { width: 1536, height: 1024 },
  shadows: { width: 1536, height: 1024 },
};

export function seedState(): LibraryState {
  return withStarters({
    version: 2,
    wallpapers: {},
    collections: [],
    settings: { ...DEFAULT_SETTINGS },
    hints: {},
  });
}

// The first prototype stored collections with their images inline:
//   { collections: [{ id, name, enabled, images: [{ id, title, src, uploaded }] }],
//     mode: "shuffle" | "order", frequency: "hour" | "three" | "day", dailyTime, paused }
interface LegacyState {
  collections: {
    id: string;
    name: string;
    enabled?: boolean;
    images?: { id: string; title?: string; src: string; uploaded?: boolean }[];
  }[];
  mode?: string;
  frequency?: string;
  dailyTime?: string;
  paused?: boolean;
}

function fromLegacy(saved: LegacyState): LibraryState {
  const wallpapers: Record<string, Wallpaper> = {};
  const collections: Collection[] = saved.collections.map((collection) => {
    const ids: string[] = [];
    for (const image of collection.images ?? []) {
      if (!image?.id || !image.src) continue;
      const sample = !image.uploaded;
      wallpapers[image.id] ??= {
        id: image.id,
        title: image.title || "Untitled",
        kind: "image",
        width: sample ? (SAMPLE_SIZES[image.id] ?? SAMPLE_SIZE).width : 0,
        height: sample ? (SAMPLE_SIZES[image.id] ?? SAMPLE_SIZE).height : 0,
        origin: sample ? "sample" : "import",
        // Earlier uploads were kept inline as data URLs; they keep working as-is.
        full: { url: image.src },
        thumb: { url: image.src },
        addedAt: 0,
      };
      ids.push(image.id);
    }
    return {
      id: collection.id,
      name: collection.name,
      inRotation: Boolean(collection.enabled),
      wallpaperIds: ids,
    };
  });
  const interval: IntervalPreset =
    saved.frequency === "three" ? "three" : saved.frequency === "day" ? "day" : "hour";
  return {
    version: 2,
    wallpapers,
    collections,
    settings: {
      ...DEFAULT_SETTINGS,
      shuffle: saved.mode !== "order",
      interval,
      dailyTime: saved.dailyTime || DEFAULT_SETTINGS.dailyTime,
      paused: Boolean(saved.paused),
    },
    hints: {},
  };
}

/** Turn whatever was saved locally into the current shape, or null when there is nothing usable. */
export function restore(saved: unknown): LibraryState | null {
  if (!saved || typeof saved !== "object") return null;
  const value = saved as Partial<LibraryState> & Partial<LegacyState>;
  if (value.version === 2 && value.wallpapers && Array.isArray(value.collections)) {
    return offerStarters({
      version: 2,
      wallpapers: Object.fromEntries(
        Object.entries(value.wallpapers).map(([id, wallpaper]) => [
          id,
          wallpaper.origin === "sample" && SAMPLE_SIZES[id]
            ? { ...wallpaper, ...SAMPLE_SIZES[id] }
            : wallpaper,
        ]),
      ),
      collections: value.collections as Collection[],
      settings: { ...DEFAULT_SETTINGS, ...value.settings },
      hints: value.hints ?? {},
      starterSet: value.starterSet,
    });
  }
  if (Array.isArray(value.collections)) return offerStarters(fromLegacy(value as LegacyState));
  return null;
}

/**
 * A library made before the high-resolution starters existed gets them once.
 * After that the person's choices stand: deleting them does not bring them back.
 */
function offerStarters(state: LibraryState): LibraryState {
  return (state.starterSet ?? 1) >= STARTER_SET ? state : withStarters(state);
}

/**
 * What may be played for a live wallpaper, or undefined for a still picture.
 * A scene kept as a file is only ever run from a name ending in .html: the Mac
 * app checks such files on arrival, and anything else must never be framed as
 * a page.
 */
export function playableRef(wallpaper: Wallpaper): Ref | undefined {
  if (wallpaper.kind === "image") return undefined;
  if (wallpaper.kind === "scene" && "file" in wallpaper.full)
    return /\.html?$/i.test(wallpaper.full.file) ? wallpaper.full : undefined;
  return wallpaper.full;
}

/**
 * The still to put on the desktop underneath a live wallpaper, if any.
 * A clock's poster shows a frozen time, so it is never used for that
 * (docs/16-live-scene-standard.md).
 */
export function underlayRef(wallpaper: Wallpaper): Ref | undefined {
  return wallpaper.tags?.includes("clock") ? undefined : wallpaper.poster;
}

export function isLive(wallpaper: Pick<Wallpaper, "kind">): boolean {
  return wallpaper.kind !== "image";
}

/** Width over height; a sensible default when the size is unknown. */
export function aspectOf(wallpaper: Pick<Wallpaper, "width" | "height">): number {
  return wallpaper.width > 0 && wallpaper.height > 0 ? wallpaper.width / wallpaper.height : 16 / 10;
}

/** Wide enough to read as one picture across two side-by-side displays. */
export const PANORAMA_ASPECT = 2.6;

export function refNames(wallpaper: Wallpaper): string[] {
  return [wallpaper.full, wallpaper.thumb, wallpaper.poster]
    .filter((ref): ref is { file: string } => Boolean(ref && "file" in ref))
    .map((ref) => ref.file);
}
