// What the page shows, by hand. The files themselves come from the catalog's
// seed folder (run `npm run media` after scenes or stills change).

/** The scene that runs across both displays in the hero. Must be a shader scene. */
export const HERO_SCENE = "low-orbit";

/** Live scenes offered in the "running right now" section, first one shown. */
export const LIVE_SCENES = ["silk", "low-orbit", "threads", "soft-glass", "daylight", "dune-wind", "arc-clock"];

/** The "one on each" state of the hero: laptop, then monitor. */
export const EACH_DISPLAY: [string, string] = ["/media/stills/black-satin.jpg", "/media/stills/alpenglow.jpg"];

/** Stills in the catalog strip, in order. Empty means every still that exists. */
export const STILLS: string[] = [];
