import { convertFileSrc, invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
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

    apply: (request: ApplyRequest) => invoke<RotationStatus>("wallpaper_apply", { request }),
    syncRotation: (plan: RotationPlan) => invoke<RotationStatus>("rotation_sync", { plan }),
    rotationCommand: (command: RotationCommand) =>
      invoke<RotationStatus>("rotation_command", { command }),
    rotationStatus: () => invoke<RotationStatus>("rotation_status"),
    onRotationChanged: (listener) => subscribe<RotationStatus>("rotation-changed", listener),
  };
}
