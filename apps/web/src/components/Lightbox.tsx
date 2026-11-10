import { useCallback, useEffect, type ReactNode } from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { ChevronLeft, ChevronRight, X } from "lucide-react";
import type { WallpaperKind } from "@/platform/types";
import { Button } from "./ui/button";
import { LiveFrame, Picture } from "./Picture";

export interface LightboxItem {
  key: string;
  title: string;
  kind: WallpaperKind;
  /** Large still. */
  image: string | undefined;
  /** Playable source for live wallpapers. */
  live?: string;
  color?: string;
  aspect: number;
  /** One quiet line under the title, e.g. "3840 × 2400, by Still Originals". */
  detail: string;
  /** Said after the detail when the picture is too small somewhere. */
  warning?: string | null;
}

/**
 * Full-size view of one wallpaper with previous/next/close.
 * (Radix-based stand-in for the paid Planes lightbox noted in the component catalog.)
 * Arrow keys move, Escape closes, focus returns to the tile that opened it.
 */
export function Lightbox({
  items,
  index,
  onIndexChange,
  onClose,
  actions,
}: {
  items: readonly LightboxItem[];
  /** null = closed. */
  index: number | null;
  onIndexChange: (index: number) => void;
  onClose: () => void;
  actions: (item: LightboxItem) => ReactNode;
}) {
  const open = index !== null && index >= 0 && index < items.length;
  const item = open ? items[index!]! : null;
  const many = items.length > 1;

  const step = useCallback(
    (delta: number) => {
      if (index === null || !many) return;
      onIndexChange((index + delta + items.length) % items.length);
    },
    [index, items.length, many, onIndexChange],
  );

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      // Leave arrow keys alone while a menu inside the viewer is open.
      if (document.querySelector("[role='menu']")) return;
      if (event.key === "ArrowLeft") step(-1);
      if (event.key === "ArrowRight") step(1);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, step]);

  return (
    <DialogPrimitive.Root open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="fixed inset-0 z-40 bg-[rgb(10_10_9/0.94)] backdrop-blur-xl data-[state=closed]:animate-fade-out data-[state=open]:animate-fade-in" />
        <DialogPrimitive.Content
          aria-describedby={undefined}
          className="fixed inset-0 z-50 flex flex-col data-[state=closed]:animate-fade-out data-[state=open]:animate-fade-in"
        >
          {item && (
            <>
              {/* The viewer covers the app's top bar, so this strip takes over
                  its job of being the place to grab and move the window. */}
              <div data-tauri-drag-region className="flex h-[52px] shrink-0 items-center justify-end px-3">
                <DialogPrimitive.Close asChild>
                  <Button variant="ghost" size="icon" aria-label="Close">
                    <X />
                  </Button>
                </DialogPrimitive.Close>
              </div>

              <div className="relative flex min-h-0 flex-1 items-center justify-center px-16">
                {many && (
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Previous wallpaper"
                    className="absolute left-3"
                    onClick={() => step(-1)}
                  >
                    <ChevronLeft />
                  </Button>
                )}
                {/* The frame keeps the wallpaper's own shape and fits both ways. */}
                <div
                  key={item.key}
                  className="relative max-h-full max-w-full animate-fade-in overflow-hidden rounded-panel shadow-[0_24px_80px_rgb(0_0_0/0.6)]"
                  style={{
                    aspectRatio: String(item.aspect),
                    width: `min(100%, calc((100vh - 52px - 96px) * ${item.aspect}))`,
                  }}
                >
                  <Picture
                    src={item.image}
                    alt={item.title}
                    color={item.color}
                    eager
                    className="size-full"
                  />
                  {item.kind !== "image" && item.live && (
                    <div className="absolute inset-0">
                      <LiveFrame kind={item.kind} src={item.live} />
                    </div>
                  )}
                </div>
                {many && (
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Next wallpaper"
                    className="absolute right-3"
                    onClick={() => step(1)}
                  >
                    <ChevronRight />
                  </Button>
                )}
              </div>

              <div className="flex h-24 shrink-0 items-center justify-between gap-6 px-6">
                <div className="min-w-0">
                  <DialogPrimitive.Title className="truncate text-lg font-semibold tracking-[-0.01em]">
                    {item.title}
                  </DialogPrimitive.Title>
                  <p className="mt-0.5 truncate text-sm text-fg-muted">
                    {item.detail}
                    {item.warning && (
                      <span className="text-danger">
                        {item.detail ? ". " : ""}
                        {item.warning}
                      </span>
                    )}
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-2">{actions(item)}</div>
              </div>
            </>
          )}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
