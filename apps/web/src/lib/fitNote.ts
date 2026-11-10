import { fitsDisplay, type FitDisplay, type FitItem } from "./fit";

/**
 * One plain sentence saying where a wallpaper would look soft, or null when it
 * is large enough for every connected display.
 */
export function fitNote(
  item: FitItem,
  displays: readonly (FitDisplay & { name: string })[],
): string | null {
  const tooSmallFor = displays.filter((display) => !fitsDisplay(item, display));
  if (tooSmallFor.length === 0) return null;
  if (displays.length === 1) return "Too small for this display";
  if (tooSmallFor.length === displays.length) return "Too small for your displays";
  return `Too small for ${tooSmallFor.map((display) => display.name).join(" and ")}`;
}
