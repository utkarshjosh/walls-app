import { convertFileSrc, invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { localDb } from "./db";
import type {
  ApplyRequest,
  DisplayInfo,
  Platform,
  RotationCommand,
  RotationPlan,
  RotationStatus,
  StoredFile,
} from "./types";

// The Mac app host. Every method is one Rust command in apps/desktop/src-tauri.
// The native side returns name/path/bytes; the URL the webview can load is
// derived here.

type NativeFile = Omit<StoredFile, "url">;

/** The library list, kept as a file beside the wallpapers it describes. */
const STATE_FILE = "library-state.json";

const withUrl = (file: NativeFile): StoredFile => ({ ...file, url: convertFileSrc(file.path) });

function subscribe<T>(event: string, listener: (payload: T) => void): () => void {
  let cancelled = false;
  let stop: (() => void) | undefined;
  listen<T>(event, (message) => listener(message.payload)).then((unlisten) => {
    if (cancelled) unlisten();
    else stop = unlisten;
  });
  return () => {
    cancelled = true;
    stop?.();
  };
}

export function createDesktopPlatform(): Platform {
  return {
    kind: "desktop",
    canSetWallpaper: true,

    displays: () => invoke<DisplayInfo[]>("displays_list"),
    onDisplaysChanged: (listener) => subscribe<DisplayInfo[]>("displays-changed", listener),

    async storeFile(name, data) {
      const bytes = new Uint8Array(await data.arrayBuffer());
      const file = await invoke<NativeFile>("library_store", bytes, {
        headers: { "x-file-name": encodeURIComponent(name) },
      });
      return withUrl(file);
    },
    async downloadFile(name, url) {
      return withUrl(await invoke<NativeFile>("library_download", { name, url }));
    },
    async removeFiles(names) {
      await invoke("library_remove", { names });
    },
    async resolveFiles(names) {
      const found = await invoke<Record<string, NativeFile>>("library_resolve", { names });
      return Object.fromEntries(Object.entries(found).map(([name, file]) => [name, withUrl(file)]));
    },

    async loadState() {
      const found = await invoke<Record<string, NativeFile>>("library_resolve", {
        names: [STATE_FILE],
      });
      const file = found[STATE_FILE];
      if (file) {
        // The timestamp keeps the web view from answering with an old copy.
        const response = await fetch(`${convertFileSrc(file.path)}?t=${Date.now()}`);
        // A list that exists but cannot be read must stop here: falling back
        // would let an empty library be saved over it.
        if (!response.ok) throw new Error("The saved library could not be read.");
        return (await response.json()) as unknown;
      }
      // Before the file existed the list lived in the web view's own storage,
      // which differs between a development run and a release build. Carry it over.
      try {
        return (await (await localDb()).get("state", "library")) ?? null;
      } catch {
        return null;
      }
    },
    async saveState(state) {
      const bytes = new TextEncoder().encode(JSON.stringify(state));
      await invoke<NativeFile>("library_store", bytes, {
        headers: { "x-file-name": encodeURIComponent(STATE_FILE) },
      });
    },

    apply: (request: ApplyRequest) => invoke<RotationStatus>("wallpaper_apply", { request }),
    syncRotation: (plan: RotationPlan) => invoke<RotationStatus>("rotation_sync", { plan }),
    rotationCommand: (command: RotationCommand) =>
      invoke<RotationStatus>("rotation_command", { command }),
    rotationStatus: () => invoke<RotationStatus>("rotation_status"),
    onRotationChanged: (listener) => subscribe<RotationStatus>("rotation-changed", listener),
  };
}
