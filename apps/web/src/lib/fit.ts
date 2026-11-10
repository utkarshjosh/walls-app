// Is a wallpaper large enough for the displays it would be shown on?
//
// A wallpaper fills its display ("cover"): it is scaled until both sides are
// covered and the overflow is cropped. If the picture has fewer pixels than the
// display, that scaling is an enlargement and the result looks soft. These
// rules decide when that is acceptable, so the app never puts a visibly blurry
// picture on the desktop by itself, and never stretches a small picture across
// several displays.
//
// The Mac app's native core applies the same rules (apps/desktop/src-tauri/src/fit.rs);
// this copy drives what the interface shows and the browser preview.
// Pure and dependency-free so `node --test` can import it directly.

export interface FitItem {
  kind: "image" | "video" | "scene";
  /** Pixels of the original. 0 (or less) means unknown. */
  width: number;
  height: number;
}

export interface FitDisplay {
  /** Position and size in points, as in the platform contract. */
  x: number;
  y: number;
  width: number;
  height: number;
  /** Pixels per point (2 on a Retina display). */
  scale: number;
}

/** A still picture may be enlarged by up to a quarter before it counts as too small. */
export const SHARP_LIMIT = 1.25;
/** Moving pictures hide softness better: 1080p video is accepted on a 16-inch Retina display, 720p is not. */
export const VIDEO_LIMIT = 2.2;

export function limitFor(kind: FitItem["kind"]): number {
  return kind === "video" ? VIDEO_LIMIT : SHARP_LIMIT;
}

/** Scenes are drawn at any size, and a picture of unknown size cannot be judged: both always pass. */
function alwaysFits(item: FitItem): boolean {
  return item.kind === "scene" || !(item.width > 0) || !(item.height > 0);
}

/**
 * How much the picture is enlarged to fill this display.
 * 1 means pixel for pixel, 2 means every picture pixel covers four display pixels.
 */
export function enlargementOn(item: FitItem, display: FitDisplay): number {
  return Math.max(
    (display.width * display.scale) / item.width,
    (display.height * display.scale) / item.height,
  );
}

/**
 * The worst enlargement on any display when one picture covers all of them as
 * a single canvas (their bounding box). A dense display next to an ordinary one
 * needs far more pixels than its share of the width suggests.
 */
export function enlargementSpanned(item: FitItem, displays: readonly FitDisplay[]): number {
  if (displays.length === 0) return 0;
  const left = Math.min(...displays.map((d) => d.x));
  const top = Math.min(...displays.map((d) => d.y));
  const right = Math.max(...displays.map((d) => d.x + d.width));
  const bottom = Math.max(...displays.map((d) => d.y + d.height));
  // Points of canvas covered by one picture pixel.
  const pointsPerPixel = Math.max((right - left) / item.width, (bottom - top) / item.height);
  return pointsPerPixel * Math.max(...displays.map((d) => d.scale));
}

export function fitsDisplay(item: FitItem, display: FitDisplay): boolean {
  return alwaysFits(item) || enlargementOn(item, display) <= limitFor(item.kind);
}

export function fitsEvery(item: FitItem, displays: readonly FitDisplay[]): boolean {
  return displays.every((display) => fitsDisplay(item, display));
}

export function fitsAny(item: FitItem, displays: readonly FitDisplay[]): boolean {
  return displays.length === 0 || displays.some((display) => fitsDisplay(item, display));
}

/** Large enough to be stretched across all displays as one picture. Needs at least two displays. */
export function canSpan(item: FitItem, displays: readonly FitDisplay[]): boolean {
  if (displays.length < 2) return false;
  return alwaysFits(item) || enlargementSpanned(item, displays) <= limitFor(item.kind);
}

/**
 * What automatic rotation does with a wallpaper when every display shows the
 * same thing ("same") or one picture is stretched across all ("span"):
 *  - "span": stretch it across the displays;
 *  - "same": show it on each display (in span mode this is the fallback for a
 *            picture that is fine per display but too small to stretch);
 *  - "skip": too small for at least one display, so it is not used.
 */
export type Placement = "span" | "same" | "skip";

export function placementFor(
  item: FitItem,
  displays: readonly FitDisplay[],
  mode: "same" | "span",
): Placement {
  if (mode === "span" && canSpan(item, displays)) return "span";
  return fitsEvery(item, displays) ? "same" : "skip";
}

/** For the one line in Rotation that says what will happen to the chosen wallpapers. */
export function rotationUse(
  items: readonly FitItem[],
  displays: readonly FitDisplay[],
  mode: "same" | "span" | "each",
): { spanned: number; shown: number; skipped: number } {
  let spanned = 0;
  let shown = 0;
  let skipped = 0;
  for (const item of items) {
    if (mode === "each") {
      // Each display draws from the wallpapers that are large enough for it.
      if (fitsAny(item, displays)) shown++;
      else skipped++;
      continue;
    }
    const placement = placementFor(item, displays, mode);
    if (placement === "span") spanned++;
    else if (placement === "same") shown++;
    else skipped++;
  }
  return { spanned, shown, skipped };
}
