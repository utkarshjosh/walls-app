import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { localDb } from "@/platform/db";
import { platform } from "@/platform";
import type {
  DisplayInfo,
  DisplayMode,
  PlayItem,
  RotationCommand,
  RotationPlan,
  RotationStatus,
  StoredFile,
} from "@/platform/types";
import { fileUrl, type CatalogItem, type CatalogPack } from "@/api/client";
import { canSpan } from "@/lib/fit";
import { prepareFile, thumbnailOf, titleFromFileName } from "@/lib/media";
import { toSchedule } from "@/lib/schedule";
import { newId } from "@/lib/utils";
import {
  SAVED_COLLECTION,
  refNames,
  restore,
  seedState,
  type Collection,
  type LibraryState,
  type Ref,
  type Settings,
  type Wallpaper,
} from "./model";

export interface ImportResult {
  name: string;
  ok: boolean;
  message: string;
}

export interface WallpaperTarget {
  mode: DisplayMode;
  /** With mode "same": only this display. */
  displayId?: string;
}

interface Library {
  state: LibraryState;
  loaded: boolean;
  /** Set when local saving fails; the app keeps working for this session. */
  storageError: string;
  displays: DisplayInfo[];
  /** The mode in effect: always "same" while fewer than two displays are attached. */
  displayMode: DisplayMode;
  status: RotationStatus;
  canSetWallpaper: boolean;

  urlOf(ref: Ref | undefined): string | undefined;
  allWallpapers: Wallpaper[];
  wallpapersIn(collectionId: string): Wallpaper[];
  /** Wallpapers that automatic changes will draw from, de-duplicated. */
  rotationWallpapers: Wallpaper[];
  savedRemoteIds: ReadonlySet<string>;
  busyRemoteIds: ReadonlySet<string>;

  createCollection(name: string): { id: string } | { error: string };
  renameCollection(id: string, name: string): { ok: true } | { error: string };
  deleteCollection(id: string): void;
  setInRotation(id: string, value: boolean): void;
  addToCollection(wallpaperId: string, collectionId: string): void;
  removeFromCollection(wallpaperId: string, collectionId: string): void;
  deleteWallpapers(ids: string[]): Promise<void>;
  importFiles(
    files: File[],
    collectionId: string,
    onProgress?: (done: number, total: number) => void,
  ): Promise<ImportResult[]>;
  saveItem(item: CatalogItem, collectionId?: string): Promise<Wallpaper>;
  unsaveItem(remoteId: string): Promise<void>;
  savePack(pack: CatalogPack, items: CatalogItem[]): Promise<string>;
  updateSettings(patch: Partial<Settings>): void;
  markHintSeen(hint: keyof LibraryState["hints"]): void;

  setWallpaper(wallpaper: Wallpaper, target?: WallpaperTarget): Promise<void>;
  command(command: RotationCommand): Promise<void>;
  exportBackup(): void;
}

const LibraryContext = createContext<Library | null>(null);

// Development only: this context cannot be swapped under a mounted screen (the
// provider and its readers would hold different copies and the screen would
// fail), so an edit to this file reloads the page instead of hot-updating it.
if (import.meta.hot) import.meta.hot.accept(() => window.location.reload());

export function useLibrary(): Library {
  const value = useContext(LibraryContext);
  if (!value) throw new Error("useLibrary must be used inside LibraryProvider");
  return value;
}

const EMPTY_STATUS: RotationStatus = {
  paused: true,
  current: [],
  nextChangeAt: null,
  canGoPrevious: false,
};

const sameName = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase();

function extensionFor(type: string): string {
  if (type.includes("png")) return "png";
  if (type.includes("webp")) return "webp";
  return "jpg";
}

export function LibraryProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<LibraryState>(seedState);
  const [loaded, setLoaded] = useState(false);
  const [storageError, setStorageError] = useState("");
  const [files, setFiles] = useState<Record<string, StoredFile>>({});
  const [displays, setDisplays] = useState<DisplayInfo[]>([]);
  const [status, setStatus] = useState<RotationStatus>(EMPTY_STATUS);
  const [busyRemoteIds, setBusyRemoteIds] = useState<ReadonlySet<string>>(new Set());

  // Async work (imports, downloads) needs the latest values without re-subscribing.
  const stateRef = useRef(state);
  stateRef.current = state;
  const filesRef = useRef(files);
  filesRef.current = files;
  const synced = useRef(false);

  const rememberFiles = useCallback((stored: (StoredFile | null | undefined)[]) => {
    const fresh = stored.filter((file): file is StoredFile => Boolean(file));
    if (fresh.length === 0) return;
    // Update the ref right away: a caller may set a wallpaper it has just
    // saved before React has re-rendered with the new files.
    filesRef.current = { ...filesRef.current, ...Object.fromEntries(fresh.map((f) => [f.name, f])) };
    setFiles((current) => {
      const next = { ...current };
      for (const file of fresh) next[file.name] = file;
      return next;
    });
  }, []);

  // ── Load ────────────────────────────────────────────────────────────────
  useEffect(() => {
    let alive = true;
    (async () => {
      let restored: LibraryState | null = null;
      try {
        restored = restore(await (await localDb()).get("state", "library"));
      } catch {
        if (alive) setStorageError("Changes can't be saved on this device right now.");
      }
      const initial = restored ?? seedState();
      const names = Object.values(initial.wallpapers).flatMap(refNames);
      let resolved: Record<string, StoredFile> = {};
      try {
        if (names.length) resolved = await platform.resolveFiles(names);
      } catch {
        /* pictures whose files are missing simply show their placeholder colour */
      }
      if (!alive) return;
      setState(initial);
      setFiles(resolved);
      setLoaded(true);
    })();
    return () => {
      alive = false;
    };
  }, []);

  // ── Save ────────────────────────────────────────────────────────────────
  useEffect(() => {
    if (!loaded) return;
    const timer = setTimeout(async () => {
      try {
        await (await localDb()).put("state", state, "library");
        setStorageError("");
      } catch {
        setStorageError("Changes can't be saved on this device right now.");
      }
    }, 250);
    return () => clearTimeout(timer);
  }, [state, loaded]);

  // ── Displays ────────────────────────────────────────────────────────────
  useEffect(() => {
    let alive = true;
    platform
      .displays()
      .then((list) => alive && setDisplays(list))
      .catch(() => {});
    const stop = platform.onDisplaysChanged(setDisplays);
    return () => {
      alive = false;
      stop();
    };
  }, []);

  // ── Rotation status ─────────────────────────────────────────────────────
  useEffect(() => {
    return platform.onRotationChanged((next) => {
      setStatus(next);
      // Pause/resume can also come from the menu bar; follow it.
      if (synced.current && next.paused !== stateRef.current.settings.paused) {
        setState((current) => ({
          ...current,
          settings: { ...current.settings, paused: next.paused },
        }));
      }
    });
  }, []);

  const urlOf = useCallback(
    (ref: Ref | undefined) => {
      if (!ref) return undefined;
      return "url" in ref ? ref.url : files[ref.file]?.url;
    },
    [files],
  );

  const allWallpapers = useMemo(
    () => Object.values(state.wallpapers).sort((a, b) => b.addedAt - a.addedAt),
    [state.wallpapers],
  );

  const wallpapersIn = useCallback(
    (collectionId: string) => {
      const collection = state.collections.find((item) => item.id === collectionId);
      if (!collection) return [];
      return collection.wallpaperIds
        .map((id) => state.wallpapers[id])
        .filter((wallpaper): wallpaper is Wallpaper => Boolean(wallpaper));
    },
    [state.collections, state.wallpapers],
  );

  const rotationWallpapers = useMemo(() => {
    const seen = new Set<string>();
    const list: Wallpaper[] = [];
    for (const collection of state.collections) {
      if (!collection.inRotation) continue;
      for (const id of collection.wallpaperIds) {
        const wallpaper = state.wallpapers[id];
        if (wallpaper && !seen.has(id)) {
          seen.add(id);
          list.push(wallpaper);
        }
      }
    }
    return list;
  }, [state.collections, state.wallpapers]);

  const savedRemoteIds = useMemo(
    () =>
      new Set(
        Object.values(state.wallpapers)
          .map((wallpaper) => wallpaper.remoteId)
          .filter((id): id is string => Boolean(id)),
      ),
    [state.wallpapers],
  );

  const displayMode: DisplayMode = displays.length > 1 ? state.settings.displayMode : "same";

  /** What the host needs to show this wallpaper, or null if its file is not ready yet. */
  const playItem = useCallback((wallpaper: Wallpaper): PlayItem | null => {
    const known = filesRef.current;
    if (platform.kind === "desktop") {
      if (!("file" in wallpaper.full)) return null;
      const full = known[wallpaper.full.file];
      if (!full) return null;
      const poster =
        wallpaper.poster && "file" in wallpaper.poster ? known[wallpaper.poster.file] : undefined;
      return {
        id: wallpaper.id,
        kind: wallpaper.kind,
        path: full.path,
        width: wallpaper.width,
        height: wallpaper.height,
        ...(poster ? { posterPath: poster.path } : {}),
      };
    }
    const url = "url" in wallpaper.full ? wallpaper.full.url : known[wallpaper.full.file]?.url;
    return url
      ? {
          id: wallpaper.id,
          kind: wallpaper.kind,
          path: url,
          width: wallpaper.width,
          height: wallpaper.height,
        }
      : null;
  }, []);

  // ── Keep the host's rotation plan current ───────────────────────────────
  const plan = useMemo<RotationPlan>(
    () => ({
      items: rotationWallpapers
        .map(playItem)
        .filter((item): item is PlayItem => Boolean(item)),
      shuffle: state.settings.shuffle,
      schedule: toSchedule(state.settings),
      paused: state.settings.paused,
      displayMode,
      pauseLiveOnBattery: state.settings.pauseLiveOnBattery,
    }),
    // `files` is a dependency because playItem reads the latest file paths.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [rotationWallpapers, state.settings, displayMode, playItem, files],
  );
  const planRef = useRef(plan);
  planRef.current = plan;
  const planKey = JSON.stringify(plan);

  useEffect(() => {
    if (!loaded) return;
    const timer = setTimeout(() => {
      platform
        .syncRotation(planRef.current)
        .then((next) => {
          synced.current = true;
          setStatus(next);
        })
        .catch(() => {});
    }, 120);
    return () => clearTimeout(timer);
  }, [planKey, loaded]);

  // ── Mac app only: copy bundled or remote pictures onto disk ─────────────
  // The native side can only put real files on the desktop.
  const copying = useRef(false);
  const copyFailed = useRef(new Set<string>());
  const [copyRound, setCopyRound] = useState(0);
  useEffect(() => {
    if (!loaded || platform.kind !== "desktop" || copying.current) return;
    const pending = Object.values(state.wallpapers).find(
      (wallpaper) => "url" in wallpaper.full && !copyFailed.current.has(wallpaper.id),
    );
    if (!pending || !("url" in pending.full)) return;
    copying.current = true;
    (async () => {
      const source = pending.full as { url: string };
      try {
        let patch: Partial<Wallpaper>;
        if (/^https?:\/\//.test(source.url) && pending.origin === "discover") {
          const thumbUrl = pending.thumb && "url" in pending.thumb ? pending.thumb.url : undefined;
          const posterUrl =
            pending.poster && "url" in pending.poster ? pending.poster.url : undefined;
          const [full, thumb, poster] = await Promise.all([
            platform.downloadFile(pending.id, source.url),
            thumbUrl ? platform.downloadFile(`${pending.id}-thumb`, thumbUrl) : null,
            posterUrl ? platform.downloadFile(`${pending.id}-poster`, posterUrl) : null,
          ]);
          rememberFiles([full, thumb, poster]);
          patch = {
            full: { file: full.name },
            ...(thumb ? { thumb: { file: thumb.name } } : {}),
            ...(poster ? { poster: { file: poster.name } } : {}),
          };
        } else {
          const blob = await (await fetch(source.url)).blob();
          const { thumb, color } = await thumbnailOf(blob);
          const full = await platform.storeFile(`${pending.id}.${extensionFor(blob.type)}`, blob);
          const small = await platform.storeFile(`${pending.id}-thumb.jpg`, thumb);
          rememberFiles([full, small]);
          patch = { full: { file: full.name }, thumb: { file: small.name }, color };
        }
        setState((current) => {
          const existing = current.wallpapers[pending.id];
          if (!existing) return current;
          return {
            ...current,
            wallpapers: { ...current.wallpapers, [pending.id]: { ...existing, ...patch } },
          };
        });
      } catch {
        // Leave it as it is: still visible in the library, retried next launch.
        copyFailed.current.add(pending.id);
      } finally {
        copying.current = false;
        setCopyRound((round) => round + 1);
      }
    })();
  }, [state.wallpapers, loaded, rememberFiles, copyRound]);

  // ── Measure pictures whose size was never recorded ──────────────────────
  // Size decides whether a wallpaper is sharp enough for a display, so data
  // from the first prototype (which did not keep it) is measured once.
  const measuring = useRef(new Set<string>());
  useEffect(() => {
    if (!loaded) return;
    for (const wallpaper of Object.values(state.wallpapers)) {
      if (wallpaper.kind !== "image" || wallpaper.width > 0 || measuring.current.has(wallpaper.id))
        continue;
      const url = urlOf(wallpaper.full);
      if (!url) continue;
      measuring.current.add(wallpaper.id);
      const probe = new Image();
      probe.onload = () => {
        const { naturalWidth: width, naturalHeight: height } = probe;
        if (!width || !height) return;
        setState((current) => {
          const existing = current.wallpapers[wallpaper.id];
          if (!existing || existing.width > 0) return current;
          return {
            ...current,
            wallpapers: { ...current.wallpapers, [wallpaper.id]: { ...existing, width, height } },
          };
        });
      };
      probe.src = url;
    }
  }, [state.wallpapers, loaded, urlOf]);

  // ── Collections ─────────────────────────────────────────────────────────
  const createCollection = useCallback((name: string) => {
    const title = name.trim();
    if (!title) return { error: "Give the collection a name." };
    if (stateRef.current.collections.some((item) => sameName(item.name, title)))
      return { error: "You already have a collection with that name." };
    const id = newId();
    setState((current) => ({
      ...current,
      collections: [...current.collections, { id, name: title, inRotation: false, wallpaperIds: [] }],
    }));
    return { id };
  }, []);

  const renameCollection = useCallback((id: string, name: string) => {
    const title = name.trim();
    if (!title) return { error: "Give the collection a name." };
    if (stateRef.current.collections.some((item) => item.id !== id && sameName(item.name, title)))
      return { error: "You already have a collection with that name." };
    setState((current) => ({
      ...current,
      collections: current.collections.map((item) =>
        item.id === id ? { ...item, name: title } : item,
      ),
    }));
    return { ok: true as const };
  }, []);

  const deleteCollection = useCallback((id: string) => {
    setState((current) => ({
      ...current,
      collections: current.collections.filter((item) => item.id !== id),
    }));
  }, []);

  const setInRotation = useCallback((id: string, value: boolean) => {
    setState((current) => ({
      ...current,
      collections: current.collections.map((item) =>
        item.id === id ? { ...item, inRotation: value } : item,
      ),
    }));
  }, []);

  const addToCollection = useCallback((wallpaperId: string, collectionId: string) => {
    setState((current) => ({
      ...current,
      collections: current.collections.map((item) =>
        item.id === collectionId && !item.wallpaperIds.includes(wallpaperId)
          ? { ...item, wallpaperIds: [...item.wallpaperIds, wallpaperId] }
          : item,
      ),
    }));
  }, []);

  const removeFromCollection = useCallback((wallpaperId: string, collectionId: string) => {
    setState((current) => ({
      ...current,
      collections: current.collections.map((item) =>
        item.id === collectionId
          ? { ...item, wallpaperIds: item.wallpaperIds.filter((id) => id !== wallpaperId) }
          : item,
      ),
    }));
  }, []);

  const deleteWallpapers = useCallback(async (ids: string[]) => {
    const doomed = new Set(ids);
    const names = ids.flatMap((id) => {
      const wallpaper = stateRef.current.wallpapers[id];
      return wallpaper ? refNames(wallpaper) : [];
    });
    setState((current) => {
      const wallpapers = { ...current.wallpapers };
      for (const id of doomed) delete wallpapers[id];
      return {
        ...current,
        wallpapers,
        collections: current.collections.map((item) => ({
          ...item,
          wallpaperIds: item.wallpaperIds.filter((id) => !doomed.has(id)),
        })),
      };
    });
    if (names.length) await platform.removeFiles(names).catch(() => {});
  }, []);

  /** Add a wallpaper and put it in a collection in one update. */
  const insert = useCallback((wallpaper: Wallpaper, collectionId: string) => {
    setState((current) => ({
      ...current,
      wallpapers: { ...current.wallpapers, [wallpaper.id]: wallpaper },
      collections: current.collections.map((item) =>
        item.id === collectionId && !item.wallpaperIds.includes(wallpaper.id)
          ? { ...item, wallpaperIds: [...item.wallpaperIds, wallpaper.id] }
          : item,
      ),
    }));
  }, []);

  const importFiles = useCallback<Library["importFiles"]>(
    async (list, collectionId, onProgress) => {
      const results: ImportResult[] = [];
      let done = 0;
      for (const file of list) {
        try {
          const prepared = await prepareFile(file);
          const id = newId();
          let stored: (StoredFile | undefined)[];
          try {
            stored = [
              await platform.storeFile(`${id}.${prepared.extension}`, file),
              await platform.storeFile(`${id}-thumb.jpg`, prepared.thumb),
              prepared.poster
                ? await platform.storeFile(`${id}-poster.jpg`, prepared.poster)
                : undefined,
            ];
          } catch {
            throw new Error("There isn't enough room to keep this file.");
          }
          rememberFiles(stored);
          const [full, thumb, poster] = stored;
          insert(
            {
              id,
              title: titleFromFileName(file.name),
              kind: prepared.kind,
              width: prepared.width,
              height: prepared.height,
              origin: "import",
              full: { file: full!.name },
              thumb: { file: thumb!.name },
              ...(poster ? { poster: { file: poster.name } } : {}),
              color: prepared.color,
              addedAt: Date.now(),
            },
            collectionId,
          );
          results.push({ name: file.name, ok: true, message: "Added" });
        } catch (error) {
          results.push({
            name: file.name,
            ok: false,
            message: error instanceof Error ? error.message : "This file could not be added.",
          });
        }
        onProgress?.(++done, list.length);
      }
      return results;
    },
    [insert, rememberFiles],
  );

  // ── Discover ────────────────────────────────────────────────────────────
  const fetchItem = useCallback(
    async (item: CatalogItem): Promise<Wallpaper> => {
      const id = `d${item.id}`;
      const urls = {
        full: fileUrl(item.files.original),
        thumb: fileUrl(item.files.thumb),
        poster: item.files.poster ? fileUrl(item.files.poster) : undefined,
      };
      let refs: Pick<Wallpaper, "full" | "thumb" | "poster">;
      if (platform.kind === "desktop") {
        const [full, thumb, poster] = await Promise.all([
          platform.downloadFile(id, urls.full),
          platform.downloadFile(`${id}-thumb`, urls.thumb),
          urls.poster ? platform.downloadFile(`${id}-poster`, urls.poster) : null,
        ]);
        rememberFiles([full, thumb, poster]);
        refs = {
          full: { file: full.name },
          thumb: { file: thumb.name },
          ...(poster ? { poster: { file: poster.name } } : {}),
        };
      } else {
        refs = {
          full: { url: urls.full },
          thumb: { url: urls.thumb },
          ...(urls.poster ? { poster: { url: urls.poster } } : {}),
        };
      }
      return {
        id,
        title: item.title,
        kind: item.kind,
        width: item.width,
        height: item.height,
        origin: "discover",
        ...refs,
        color: item.color,
        author: item.author,
        license: item.license,
        sourceUrl: item.sourceUrl,
        remoteId: item.id,
        addedAt: Date.now(),
      };
    },
    [rememberFiles],
  );

  /** The collection plain "Save" adds to, created (in rotation) the first time it is needed. */
  const savedCollectionId = useCallback(() => {
    const existing = stateRef.current.collections.find((item) =>
      sameName(item.name, SAVED_COLLECTION),
    );
    if (existing) return existing.id;
    const id = newId();
    const fresh: Collection = { id, name: SAVED_COLLECTION, inRotation: true, wallpaperIds: [] };
    stateRef.current = {
      ...stateRef.current,
      collections: [...stateRef.current.collections, fresh],
    };
    setState((current) =>
      current.collections.some((item) => item.id === id)
        ? current
        : { ...current, collections: [...current.collections, fresh] },
    );
    return id;
  }, []);

  const markBusy = useCallback((remoteId: string, busy: boolean) => {
    setBusyRemoteIds((current) => {
      const next = new Set(current);
      if (busy) next.add(remoteId);
      else next.delete(remoteId);
      return next;
    });
  }, []);

  const saveItem = useCallback<Library["saveItem"]>(
    async (item, collectionId) => {
      const target = collectionId ?? savedCollectionId();
      const existing = Object.values(stateRef.current.wallpapers).find(
        (wallpaper) => wallpaper.remoteId === item.id,
      );
      if (existing) {
        addToCollection(existing.id, target);
        return existing;
      }
      markBusy(item.id, true);
      try {
        const wallpaper = await fetchItem(item);
        insert(wallpaper, target);
        return wallpaper;
      } finally {
        markBusy(item.id, false);
      }
    },
    [addToCollection, fetchItem, insert, markBusy, savedCollectionId],
  );

  const unsaveItem = useCallback(
    async (remoteId: string) => {
      const wallpaper = Object.values(stateRef.current.wallpapers).find(
        (candidate) => candidate.remoteId === remoteId,
      );
      if (wallpaper) await deleteWallpapers([wallpaper.id]);
    },
    [deleteWallpapers],
  );

  const savePack = useCallback<Library["savePack"]>(
    async (pack, items) => {
      let collection = stateRef.current.collections.find(
        (item) => item.packId === pack.id || sameName(item.name, pack.name),
      );
      if (!collection) {
        collection = {
          id: newId(),
          name: pack.name,
          inRotation: false,
          wallpaperIds: [],
          packId: pack.id,
        };
        const fresh = collection;
        stateRef.current = {
          ...stateRef.current,
          collections: [...stateRef.current.collections, fresh],
        };
        setState((current) => ({ ...current, collections: [...current.collections, fresh] }));
      }
      const targetId = collection.id;
      // A few at a time: quick, without hammering the connection.
      const queue = [...items];
      const worker = async () => {
        for (let item = queue.shift(); item; item = queue.shift()) {
          await saveItem(item, targetId).catch(() => {});
        }
      };
      await Promise.all([worker(), worker(), worker()]);
      return targetId;
    },
    [saveItem],
  );

  // ── Settings and rotation ───────────────────────────────────────────────
  const updateSettings = useCallback((patch: Partial<Settings>) => {
    setState((current) => ({ ...current, settings: { ...current.settings, ...patch } }));
  }, []);

  const markHintSeen = useCallback<Library["markHintSeen"]>((hint) => {
    setState((current) =>
      current.hints[hint] ? current : { ...current, hints: { ...current.hints, [hint]: true } },
    );
  }, []);

  const command = useCallback<Library["command"]>(
    async (name) => {
      if (name === "pause" || name === "resume") {
        const paused = name === "pause";
        // Tell the host first so "resume" sees an up-to-date plan.
        await platform.syncRotation({ ...planRef.current, paused });
        updateSettings({ paused });
      }
      setStatus(await platform.rotationCommand(name));
    },
    [updateSettings],
  );

  const setWallpaper = useCallback<Library["setWallpaper"]>(
    async (wallpaper, target) => {
      const item = playItem(stateRef.current.wallpapers[wallpaper.id] ?? wallpaper);
      if (!item) throw new Error("This wallpaper is still being prepared. Try again in a moment.");
      const multi = displays.length > 1;
      // Never stretch a picture that is too small for it; show it on each display instead.
      if (target?.mode === "span" && !canSpan(wallpaper, displays)) target = { mode: "same" };
      if (multi && target) {
        // Choosing where a wallpaper goes is also choosing how the displays are
        // used from now on: everywhere, across all, or one display on its own.
        const displayMode: DisplayMode = target.displayId ? "each" : target.mode;
        if (displayMode !== stateRef.current.settings.displayMode) {
          await platform.syncRotation({ ...planRef.current, displayMode });
          updateSettings({ displayMode });
        }
      }
      const mode = target?.mode ?? (multi ? stateRef.current.settings.displayMode : "same");
      setStatus(
        await platform.apply({
          // "Different on each display" needs several pictures; one picked by hand goes everywhere.
          mode: mode === "each" && !target ? "same" : mode,
          items: [item],
          ...(target?.displayId ? { displayId: target.displayId } : {}),
        }),
      );
    },
    [displays, playItem, updateSettings],
  );

  const exportBackup = useCallback(() => {
    const link = document.createElement("a");
    link.href = URL.createObjectURL(
      new Blob([JSON.stringify(stateRef.current, null, 2)], { type: "application/json" }),
    );
    link.download = "still-library-backup.json";
    link.click();
    setTimeout(() => URL.revokeObjectURL(link.href), 1000);
  }, []);

  const value: Library = {
    state,
    loaded,
    storageError,
    displays,
    displayMode,
    status,
    canSetWallpaper: platform.canSetWallpaper,
    urlOf,
    allWallpapers,
    wallpapersIn,
    rotationWallpapers,
    savedRemoteIds,
    busyRemoteIds,
    createCollection,
    renameCollection,
    deleteCollection,
    setInRotation,
    addToCollection,
    removeFromCollection,
    deleteWallpapers,
    importFiles,
    saveItem,
    unsaveItem,
    savePack,
    updateSettings,
    markHintSeen,
    setWallpaper,
    command,
    exportBackup,
  };

  return <LibraryContext.Provider value={value}>{children}</LibraryContext.Provider>;
}
