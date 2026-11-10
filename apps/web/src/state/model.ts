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

const SAMPLE_SIZE = { width: 1672, height: 941 };
/** Four of the bundled samples are 3:2 rather than 16:9. */
const SAMPLE_SIZES: Record<string, { width: number; height: number }> = {
  coast: { width: 1536, height: 1024 },
  "coastal-flight": { width: 1536, height: 1024 },
  ridges: { width: 1536, height: 1024 },
  shadows: { width: 1536, height: 1024 },
};

const SAMPLES: { id: string; name: string; inRotation: boolean; images: [string, string][] }[] = [
  {
    id: "aviation",
    name: "Aviation",
    inRotation: true,
    images: [
      ["glider", "Above the clouds"],
      ["wing", "Golden hour"],
      ["coastal-flight", "Coastal flight"],
    ],
  },
  {
    id: "landscapes",
    name: "Landscapes",
    inRotation: true,
    images: [
      ["alpine", "Alpine light"],
      ["coast", "Black sand coast"],
      ["ridges", "Morning ridges"],
    ],
  },
  {
    id: "abstract",
    name: "Abstract",
    inRotation: false,
    images: [
      ["paper", "Paper forms"],
      ["limestone", "Quiet stone"],
      ["shadows", "Light and shadow"],
    ],
  },
];

function sampleWallpaper(id: string, title: string): Wallpaper {
  const url = `/wallpapers/${id}.jpg`;
  return {
    id,
    title,
    kind: "image",
    ...(SAMPLE_SIZES[id] ?? SAMPLE_SIZE),
    origin: "sample",
    full: { url },
    thumb: { url },
    addedAt: 0,
  };
}

export function seedState(): LibraryState {
  const wallpapers: Record<string, Wallpaper> = {};
  const collections = SAMPLES.map((sample) => {
    for (const [id, title] of sample.images) wallpapers[id] = sampleWallpaper(id, title);
    return {
      id: sample.id,
      name: sample.name,
      inRotation: sample.inRotation,
      wallpaperIds: sample.images.map(([id]) => id),
    };
  });
  return { version: 2, wallpapers, collections, settings: { ...DEFAULT_SETTINGS }, hints: {} };
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
    return {
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
    };
  }
  if (Array.isArray(value.collections)) return fromLegacy(value as LegacyState);
  return null;
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
