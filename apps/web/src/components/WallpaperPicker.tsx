import { useEffect, useMemo, useState } from "react";
import { fitsDisplay } from "@/lib/fit";
import { cn } from "@/lib/utils";
import type { DisplayInfo } from "@/platform/types";
import { useLibrary } from "@/state/store";
import { DisplayIcon } from "./DisplayPreview";
import { Picture } from "./Picture";
import { Dialog, DialogContent } from "./ui/dialog";
import { useSetWallpaper } from "./useSetWallpaper";
import { Badge } from "./WallpaperGrid";

const ALL = "all";

/**
 * Choose the wallpaper for one particular display.
 * Wallpapers large enough for that display come first; smaller ones are still
 * offered, marked, because this is a choice made by hand.
 */
export function WallpaperPicker({
  display,
  onClose,
}: {
  /** The display being chosen for; null keeps the dialog closed. */
  display: DisplayInfo | null;
  onClose: () => void;
}) {
  const { state, allWallpapers, wallpapersIn, urlOf, status } = useLibrary();
  const { set, busy } = useSetWallpaper();
  const [collectionId, setCollectionId] = useState(ALL);
  // Keep the last display while the dialog fades out, so its title does not blank.
  const [shown, setShown] = useState(display);
  useEffect(() => {
    if (display) {
      setShown(display);
      setCollectionId(ALL);
    }
  }, [display]);

  const collections = state.collections.filter((collection) => collection.wallpaperIds.length > 0);
  const onDisplay = status.current.find((entry) => entry.displayId === shown?.id)?.itemId;

  const wallpapers = useMemo(() => {
    const list = collectionId === ALL ? allWallpapers : wallpapersIn(collectionId);
    if (!shown) return list;
    // Stable sort: what fits this display first, the rest after.
    return [...list].sort(
      (a, b) => Number(fitsDisplay(b, shown)) - Number(fitsDisplay(a, shown)),
    );
  }, [collectionId, allWallpapers, wallpapersIn, shown]);

  return (
    <Dialog open={Boolean(display)} onOpenChange={(next) => !next && onClose()}>
      <DialogContent
        className="w-[min(760px,calc(100vw-32px))]"
        title={
          <span className="flex items-center gap-2">
            {shown && <DisplayIcon display={shown} className="size-[18px] text-fg-muted" />}
            Wallpaper for {shown?.name ?? "this display"}
          </span>
        }
      >
        {collections.length > 1 && (
          <div className="no-scrollbar -mx-1 mb-3 flex gap-1.5 overflow-x-auto px-1 pb-1">
            {[{ id: ALL, name: "All" }, ...collections].map((collection) => (
              <button
                key={collection.id}
                type="button"
                aria-pressed={collection.id === collectionId}
                onClick={() => setCollectionId(collection.id)}
                className={cn(
                  "h-7 shrink-0 rounded-full px-3 text-sm transition-colors duration-100",
                  collection.id === collectionId
                    ? "bg-fg font-medium text-bg"
                    : "bg-raised text-fg-muted hover:bg-hover hover:text-fg",
                )}
              >
                {collection.name}
              </button>
            ))}
          </div>
        )}

        {wallpapers.length === 0 ? (
          <p className="py-10 text-center text-sm text-fg-muted">This collection is empty.</p>
        ) : (
          <div className="scroll-area -mr-2 grid max-h-[min(56vh,460px)] grid-cols-3 gap-2 pr-2 sm:grid-cols-4">
            {wallpapers.map((wallpaper) => {
              const fits = !shown || fitsDisplay(wallpaper, shown);
              const current = wallpaper.id === onDisplay;
              return (
                <button
                  key={wallpaper.id}
                  type="button"
                  disabled={busy}
                  aria-label={`Use ${wallpaper.title}${fits ? "" : " (too small for this display)"}`}
                  aria-current={current}
                  onClick={async () => {
                    if (!shown) return;
                    await set(wallpaper, { mode: "same", displayId: shown.id });
                    onClose();
                  }}
                  className={cn(
                    "group/pick relative aspect-[16/10] overflow-hidden rounded-[8px] bg-raised focus-visible:outline-offset-[-2px]",
                    current && "shadow-[inset_0_0_0_2px_var(--tint)]",
                  )}
                >
                  <Picture
                    src={urlOf(wallpaper.thumb)}
                    alt=""
                    color={wallpaper.color}
                    className={cn("size-full", !fits && "opacity-45", current && "p-0.5")}
                    imageClassName={cn(
                      "transition-[opacity,transform] duration-300 ease-quiet group-hover/pick:scale-[1.04]",
                      current && "rounded-[6px]",
                    )}
                  />
                  <span className="pointer-events-none absolute top-1.5 left-1.5 flex gap-1">
                    {wallpaper.kind !== "image" && <Badge>Live</Badge>}
                    {!fits && <Badge>Too small</Badge>}
                    {current && <Badge tone="tint">Showing</Badge>}
                  </span>
                  <span className="pointer-events-none absolute inset-x-0 bottom-0 truncate bg-gradient-to-t from-black/70 to-transparent px-2 pt-6 pb-1.5 text-left text-xs font-medium text-white opacity-0 transition-opacity duration-150 group-hover/pick:opacity-100 group-focus-visible/pick:opacity-100">
                    {wallpaper.title}
                  </span>
                </button>
              );
            })}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
