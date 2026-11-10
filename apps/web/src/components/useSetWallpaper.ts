import { useCallback, useState } from "react";
import { useLibrary, type WallpaperTarget } from "@/state/store";
import type { Wallpaper } from "@/state/model";
import { useToast } from "./Toast";

/**
 * "Set as wallpaper" with honest feedback: in the Mac app the desktop changes;
 * in a browser tab only the in-app preview does, and the message says so.
 */
export function useSetWallpaper() {
  const { setWallpaper, canSetWallpaper } = useLibrary();
  const notify = useToast();
  const [busy, setBusy] = useState(false);

  const set = useCallback(
    async (source: Wallpaper | (() => Promise<Wallpaper>), target?: WallpaperTarget) => {
      setBusy(true);
      try {
        const wallpaper = typeof source === "function" ? await source() : source;
        await setWallpaper(wallpaper, target);
        notify(
          canSetWallpaper
            ? "Wallpaper set"
            : "Showing in the Rotation preview. The Mac app sets your real desktop.",
        );
      } catch (error) {
        notify(
          typeof error === "string"
            ? error
            : error instanceof Error
              ? error.message
              : "The wallpaper could not be set.",
          { tone: "error" },
        );
      } finally {
        setBusy(false);
      }
    },
    [canSetWallpaper, notify, setWallpaper],
  );

  return { set, busy };
}
