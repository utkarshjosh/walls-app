import { cn } from "@/lib/utils";
import type { DisplayInfo, DisplayMode } from "@/platform/types";

/** Left-to-right, then top-to-bottom: the order "different on each" assigns pictures in. */
export function orderedDisplays(displays: readonly DisplayInfo[]): DisplayInfo[] {
  return [...displays].sort((a, b) => a.x - b.x || a.y - b.y);
}

export interface PreviewPicture {
  src: string | undefined;
  color?: string;
}

/**
 * The person's real display arrangement, drawn to scale, with wallpapers on it.
 * This is how the app shows what "span" or "different on each" will look like
 * before anything touches the desktop.
 */
export function DisplayPreview({
  displays,
  mode,
  pictures,
  className,
  highlightId,
  onPick,
}: {
  displays: readonly DisplayInfo[];
  mode: DisplayMode;
  /** One picture for same/span; one per display (left to right) for each. */
  pictures: readonly PreviewPicture[];
  className?: string;
  /** Emphasise one display and dim the rest. */
  highlightId?: string | null;
  /** Makes each display a button. */
  onPick?: (display: DisplayInfo) => void;
}) {
  if (displays.length === 0) return null;
  const ordered = orderedDisplays(displays);
  const minX = Math.min(...ordered.map((d) => d.x));
  const minY = Math.min(...ordered.map((d) => d.y));
  const width = Math.max(...ordered.map((d) => d.x + d.width)) - minX;
  const height = Math.max(...ordered.map((d) => d.y + d.height)) - minY;
  const spanning = mode === "span" && ordered.length > 1;

  return (
    <div
      className={cn("relative w-full", className)}
      style={{ aspectRatio: `${width} / ${height}` }}
    >
      {ordered.map((display, index) => {
        const picture =
          mode === "each" ? pictures[index % Math.max(1, pictures.length)] : pictures[0];
        const dimmed = highlightId != null && highlightId !== display.id;
        const Frame = onPick ? "button" : "div";
        return (
          <Frame
            key={display.id}
            {...(onPick
              ? {
                  type: "button" as const,
                  onClick: () => onPick(display),
                  "aria-label": `Set only on ${display.name}`,
                }
              : {})}
            className={cn(
              "absolute block overflow-hidden rounded-[6px] bg-raised shadow-[0_0_0_2px_var(--color-bg),0_8px_24px_rgb(0_0_0/0.35)] transition-opacity duration-150",
              dimmed && "opacity-35",
            )}
            style={{
              left: `${((display.x - minX) / width) * 100}%`,
              top: `${((display.y - minY) / height) * 100}%`,
              width: `${(display.width / width) * 100}%`,
              height: `${(display.height / height) * 100}%`,
              backgroundColor: picture?.color,
            }}
          >
            {picture?.src && (
              <img
                src={picture.src}
                alt=""
                draggable={false}
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
          </Frame>
        );
      })}
    </div>
  );
}

/** Tiny diagrams for the three display modes, drawn from the same idea as the preview. */
export function ModeGlyph({ mode, className }: { mode: DisplayMode; className?: string }) {
  const tones =
    mode === "same"
      ? ["a", "a"]
      : mode === "each"
        ? ["a", "b"]
        : (["span-left", "span-right"] as const);
  return (
    <svg viewBox="0 0 44 18" className={cn("h-[18px] w-11", className)} aria-hidden>
      <defs>
        <linearGradient id="glyph-span" x1="0" x2="1">
          <stop offset="0" stopColor="currentColor" stopOpacity="0.95" />
          <stop offset="1" stopColor="currentColor" stopOpacity="0.3" />
        </linearGradient>
      </defs>
      {mode === "span" ? (
        <>
          <rect x="1" y="1" width="20" height="16" rx="2.5" fill="url(#glyph-span)" />
          <rect
            x="23"
            y="1"
            width="20"
            height="16"
            rx="2.5"
            fill="currentColor"
            fillOpacity="0.3"
          />
          <path
            d="M4 13 C 14 3, 26 15, 40 5"
            stroke="var(--color-surface)"
            strokeWidth="1.6"
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
              fillOpacity={tone === "a" ? 0.9 : 0.35}
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
