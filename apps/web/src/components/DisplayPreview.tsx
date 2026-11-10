import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { Laptop, Monitor } from "lucide-react";
import { cn } from "@/lib/utils";
import type { DisplayInfo, DisplayMode } from "@/platform/types";
import { LiveFrame } from "./Picture";

/** Left-to-right, then top-to-bottom: the order "different on each" assigns pictures in. */
export function orderedDisplays(displays: readonly DisplayInfo[]): DisplayInfo[] {
  return [...displays].sort((a, b) => a.x - b.x || a.y - b.y);
}

/** A laptop's own screen, as opposed to a monitor plugged into it. */
export function isBuiltIn(display: Pick<DisplayInfo, "name">): boolean {
  return /built-?in|color lcd|macbook/i.test(display.name);
}

export function DisplayIcon({
  display,
  className,
}: {
  display: Pick<DisplayInfo, "name">;
  className?: string;
}) {
  const Icon = isBuiltIn(display) ? Laptop : Monitor;
  return <Icon className={className} aria-hidden />;
}

export interface PreviewPicture {
  src: string | undefined;
  color?: string;
  /** A scene's own file: when given, the scene runs in the display tile. */
  live?: string;
}

/**
 * The person's real display arrangement, drawn to scale, with wallpapers on it.
 * This is how the app shows what "one across all" or "different on each" will
 * look like before anything touches the desktop.
 *
 * With `contain` it fits inside the box its parent gives it in both
 * directions, so displays side by side and displays stacked take up the same
 * room on screen. Without it, it fills the available width.
 */
export function DisplayPreview({
  displays,
  layout,
  picture,
  pictureByDisplay,
  className,
  highlightId,
  labels,
  action,
  contain,
}: {
  displays: readonly DisplayInfo[];
  /** How the pictures are laid out right now (may differ from the chosen mode). */
  layout: DisplayMode;
  /** The one picture, for "same" and "span". */
  picture?: PreviewPicture;
  /** One picture per display id, for "each". A display with none stays empty. */
  pictureByDisplay?: Readonly<Record<string, PreviewPicture | undefined>>;
  className?: string;
  /** Emphasise one display and dim the rest. */
  highlightId?: string | null;
  /** Show each display's name with a laptop or monitor icon. */
  labels?: boolean;
  /** Extra control drawn on top of each display (e.g. "Choose"). */
  action?: (display: DisplayInfo) => ReactNode;
  /** Fit inside the parent's height as well as its width. The parent must have a height. */
  contain?: boolean;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [room, setRoom] = useState({ width: 0, height: 0 });
  const reduceMotion = useReducedMotion();

  useLayoutEffect(() => {
    const element = box.current;
    if (!element) return;
    const measure = () => setRoom({ width: element.clientWidth, height: element.clientHeight });
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const ordered = orderedDisplays(displays);
  const minX = Math.min(...ordered.map((d) => d.x));
  const minY = Math.min(...ordered.map((d) => d.y));
  const width = Math.max(...ordered.map((d) => d.x + d.width)) - minX;
  const height = Math.max(...ordered.map((d) => d.y + d.height)) - minY;
  // Pixels on screen per point of desktop: the largest scale at which the
  // whole arrangement fits.
  const scale =
    ordered.length === 0 || room.width === 0 || (contain && room.height === 0)
      ? 0
      : Math.min(room.width / width, contain ? room.height / height : Infinity);
  const spanning = layout === "span" && ordered.length > 1;

  return (
    <div
      ref={box}
      className={cn("flex w-full items-center justify-center", contain && "h-full", className)}
    >
      {scale > 0 && (
        <div className="relative shrink-0" style={{ width: width * scale, height: height * scale }}>
          {ordered.map((display) => {
            const shown = layout === "each" ? pictureByDisplay?.[display.id] : picture;
            const dimmed = highlightId != null && highlightId !== display.id;
            const tileWidth = display.width * scale;
            return (
              <div
                key={display.id}
                className={cn(
                  "group/display absolute overflow-hidden rounded-[6px] bg-raised shadow-[0_0_0_2px_var(--color-bg),0_8px_24px_rgb(0_0_0/0.35)] transition-opacity duration-150",
                  dimmed && "opacity-35",
                )}
                style={{
                  left: (display.x - minX) * scale,
                  top: (display.y - minY) * scale,
                  width: tileWidth,
                  height: display.height * scale,
                }}
              >
                {/* A new wallpaper fades in over the old one. */}
                <AnimatePresence initial={false}>
                  {shown?.src && (
                    <motion.img
                      key={`${shown.src}|${spanning}`}
                      src={shown.src}
                      alt=""
                      draggable={false}
                      initial={{ opacity: 0 }}
                      animate={{ opacity: 1 }}
                      exit={{ opacity: 0 }}
                      transition={{ duration: 0.35 }}
                      className="absolute max-w-none object-cover"
                      style={
                        spanning
                          ? {
                              // One picture covering the whole arrangement; each
                              // display is a window onto its own part of it.
                              width: `${(width / display.width) * 100}%`,
                              height: `${(height / display.height) * 100}%`,
                              left: `${(-(display.x - minX) / display.width) * 100}%`,
                              top: `${(-(display.y - minY) / display.height) * 100}%`,
                            }
                          : { inset: 0, width: "100%", height: "100%" }
                      }
                    />
                  )}
                </AnimatePresence>

                {/* A scene runs here exactly as it does on the desktop: given
                    the same geometry, so a stretched scene lines up across the
                    tiles and a per-display scene is whole in each. */}
                {shown?.live && !reduceMotion && (
                  <span key={shown.live} className="absolute inset-0 animate-fade-in">
                    <LiveFrame
                      kind="scene"
                      src={
                        spanning
                          ? `${shown.live}#cw=${Math.round(width * scale)}&ch=${Math.round(height * scale)}&vx=${Math.round((display.x - minX) * scale)}&vy=${Math.round((display.y - minY) * scale)}`
                          : shown.live
                      }
                    />
                  </span>
                )}

                {labels && (
                  <span
                    className="pointer-events-none absolute bottom-1.5 left-1.5 flex max-w-[calc(100%-12px)] items-center gap-1 rounded-full bg-black/60 px-1.5 py-0.5 text-2xs font-medium text-white"
                    title={display.name}
                  >
                    <DisplayIcon display={display} className="size-3 shrink-0" />
                    {/* On a very small tile the icon alone has to do. */}
                    {tileWidth >= 150 && <span className="truncate">{display.name}</span>}
                  </span>
                )}

                {action && (
                  <span className="absolute inset-0 flex items-center justify-center opacity-0 transition-opacity duration-150 group-focus-within/display:opacity-100 group-hover/display:opacity-100">
                    {action(display)}
                  </span>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

/** Tiny diagrams for the three display modes, drawn from the same idea as the preview. */
export function ModeGlyph({ mode, className }: { mode: DisplayMode; className?: string }) {
  const tones = mode === "same" ? ["a", "a"] : ["a", "b"];
  return (
    <svg viewBox="0 0 44 18" className={cn("h-[18px] w-11", className)} aria-hidden>
      {mode === "span" ? (
        <>
          <rect x="1" y="1" width="20" height="16" rx="2.5" fill="currentColor" fillOpacity="0.9" />
          <rect x="23" y="1" width="20" height="16" rx="2.5" fill="currentColor" fillOpacity="0.9" />
          {/* One line running through both screens: one picture across them. */}
          <path
            d="M4 13 C 14 3, 28 15, 40 5"
            stroke="var(--color-surface)"
            strokeWidth="1.8"
            fill="none"
            strokeLinecap="round"
          />
        </>
      ) : (
        tones.map((tone, index) => (
          <g key={index}>
            <rect
              x={1 + index * 22}
              y="1"
              width="20"
              height="16"
              rx="2.5"
              fill="currentColor"
              fillOpacity={tone === "a" ? 0.9 : 0.4}
            />
            <circle
              cx={(tone === "a" ? 7 : 15) + index * 22}
              cy={tone === "a" ? 7 : 11}
              r="2.4"
              fill="var(--color-surface)"
            />
          </g>
        ))
      )}
    </svg>
  );
}
