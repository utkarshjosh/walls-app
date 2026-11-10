import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useReducedMotion } from "motion/react";
import { cn } from "@/lib/utils";
import { platform } from "@/platform";
import { PANORAMA_ASPECT } from "@/state/model";
import type { WallpaperKind } from "@/platform/types";
import { LiveFrame, Picture } from "./Picture";

const GAP = 14;
const TILE_ASPECT = 16 / 10;

/**
 * A stable grid of equal 16:10 tiles; panoramas take two columns so wide
 * pictures look wide. Column count follows the width, and nothing animates
 * when it changes (per the motion guide: never animate gallery layout).
 *
 * This is the "adaptive density grid" pick reduced to what we use from it:
 * fixed density, no controls, no reflow animation.
 */
export function WallpaperGrid<T>({
  items,
  keyOf,
  aspectOf,
  children,
  minTile = 230,
  className,
}: {
  items: readonly T[];
  keyOf: (item: T) => string;
  aspectOf: (item: T) => number;
  children: (item: T, index: number) => ReactNode;
  minTile?: number;
  className?: string;
}) {
  const grid = useRef<HTMLDivElement>(null);
  const [metrics, setMetrics] = useState({ columns: 4, row: 150 });

  useLayoutEffect(() => {
    const element = grid.current;
    if (!element) return;
    const measure = () => {
      const width = element.clientWidth;
      if (!width) return;
      const columns = Math.max(1, Math.floor((width + GAP) / (minTile + GAP)));
      const tile = (width - GAP * (columns - 1)) / columns;
      setMetrics({ columns, row: Math.round(tile / TILE_ASPECT) });
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [minTile]);

  return (
    <div
      ref={grid}
      className={cn("grid grid-flow-dense", className)}
      style={{
        gap: GAP,
        gridTemplateColumns: `repeat(${metrics.columns}, minmax(0, 1fr))`,
        gridAutoRows: metrics.row,
      }}
    >
      {items.map((item, index) => (
        <div
          key={keyOf(item)}
          className={cn(
            "min-w-0",
            aspectOf(item) >= PANORAMA_ASPECT && metrics.columns > 1 && "col-span-2",
          )}
        >
          {children(item, index)}
        </div>
      ))}
    </div>
  );
}

/** One wallpaper in a grid. Opening is the tile itself; extra actions sit on top. */
export function WallpaperTile({
  title,
  caption,
  thumb,
  color,
  kind,
  liveSrc,
  onOpen,
  actions,
  pinnedActions,
  showing,
}: {
  title: string;
  caption?: string;
  thumb: string | undefined;
  color?: string;
  kind: WallpaperKind;
  /** When given, the live wallpaper plays while the pointer rests on the tile. */
  liveSrc?: string;
  onOpen: () => void;
  /** Controls revealed on hover or keyboard focus (top right). */
  actions?: ReactNode;
  /** Controls that stay visible, e.g. a "Saved" mark. */
  pinnedActions?: ReactNode;
  /** This wallpaper is on the desktop right now. */
  showing?: boolean;
}) {
  const reduceMotion = useReducedMotion();
  const [playing, setPlaying] = useState(false);
  const hoverTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(hoverTimer.current), []);

  const live = kind !== "image";
  const startPreview = () => {
    if (!live || !liveSrc || reduceMotion) return;
    hoverTimer.current = setTimeout(() => setPlaying(true), 350);
  };
  const stopPreview = () => {
    clearTimeout(hoverTimer.current);
    setPlaying(false);
  };

  return (
    <div
      className="group/tile relative size-full overflow-hidden rounded-tile bg-raised"
      onPointerEnter={startPreview}
      onPointerLeave={stopPreview}
    >
      <button
        type="button"
        onClick={onOpen}
        aria-label={`Open ${title}`}
        className="absolute inset-0 block size-full focus-visible:outline-offset-[-2px]"
      >
        <Picture src={thumb} alt="" color={color} className="size-full" />
        {playing && liveSrc && (
          <span className="absolute inset-0 animate-fade-in">
            <LiveFrame kind={kind} src={liveSrc} />
          </span>
        )}
        {/* Name appears on hover/focus only; the picture is the content. */}
        <span className="pointer-events-none absolute inset-x-0 bottom-0 flex items-end bg-gradient-to-t from-black/70 to-transparent px-3 pt-10 pb-2.5 text-left opacity-0 transition-opacity duration-150 group-focus-within/tile:opacity-100 group-hover/tile:opacity-100">
          <span className="min-w-0">
            <span className="block truncate text-sm font-medium text-white">{title}</span>
            {caption && <span className="block truncate text-xs text-white/70">{caption}</span>}
          </span>
        </span>
      </button>

      {(live || showing) && (
        <span className="pointer-events-none absolute top-2.5 left-2.5 flex gap-1.5">
          {live && <Badge>Live</Badge>}
          {/* A browser tab only has the in-app preview; do not claim the real desktop. */}
          {showing && (
            <Badge tone="tint">{platform.canSetWallpaper ? "On your desktop" : "In preview"}</Badge>
          )}
        </span>
      )}

      {(actions || pinnedActions) && (
        <span className="absolute top-2 right-2 flex gap-1.5">
          {actions && (
            <span className="flex gap-1.5 opacity-0 transition-opacity duration-150 group-focus-within/tile:opacity-100 group-hover/tile:opacity-100 has-[[data-state=open]]:opacity-100">
              {actions}
            </span>
          )}
          {pinnedActions}
        </span>
      )}
    </div>
  );
}

export function Badge({ children, tone }: { children: ReactNode; tone?: "tint" }) {
  return (
    <span
      className={cn(
        "rounded-full px-2 py-0.5 text-2xs font-medium backdrop-blur-md",
        tone === "tint" ? "bg-tint text-tint-ink" : "bg-black/50 text-white",
      )}
    >
      {children}
    </span>
  );
}

export function GridSkeleton({ count = 8 }: { count?: number }) {
  return (
    <div
      className="grid gap-3.5"
      style={{ gridTemplateColumns: "repeat(auto-fill, minmax(230px, 1fr))" }}
      aria-hidden
    >
      {Array.from({ length: count }, (_, index) => (
        <div key={index} className="skeleton aspect-[16/10] rounded-tile" />
      ))}
    </div>
  );
}
