import { createDesktopPlatform } from "./desktop";
import { createWebPlatform, type WebPlatform } from "./web";
import type { Platform } from "./types";

export type * from "./types";
export type { WebPlatform };

const insideTauri = typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;

/** The one host this copy of the UI is running in. */
export const platform: Platform | WebPlatform = insideTauri
  ? createDesktopPlatform()
  : createWebPlatform();

export const isMac = typeof navigator !== "undefined" && /Mac/i.test(navigator.userAgent);

export function isWebPlatform(value: Platform): value is WebPlatform {
  return value.kind === "web";
}
