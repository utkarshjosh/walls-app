// Reading files a person adds: check them, measure them, and make the small
// previews the grid uses. Runs the same way in the browser and in the Mac app.

import type { WallpaperKind } from "@/platform/types";

export const IMAGE_TYPES = ["image/jpeg", "image/png", "image/webp"];
export const VIDEO_TYPES = ["video/mp4", "video/quicktime", "video/webm"];
export const ACCEPT = [...IMAGE_TYPES, ...VIDEO_TYPES].join(",");

const MIB = 1024 * 1024;
export const MAX_IMAGE_BYTES = 25 * MIB;
export const MAX_VIDEO_BYTES = 200 * MIB;
export const MAX_PIXELS = 60_000_000;

const THUMB_WIDTH = 640;
const POSTER_WIDTH = 1920;

export interface Prepared {
  kind: WallpaperKind;
  width: number;
  height: number;
  extension: string;
  thumb: Blob;
  /** Videos only: a still frame used in the library and under the playing video. */
  poster?: Blob;
  color: string;
}

const EXTENSIONS: Record<string, string> = {
  "image/jpeg": "jpg",
  "image/png": "png",
  "image/webp": "webp",
  "video/mp4": "mp4",
  "video/quicktime": "mov",
  "video/webm": "webm",
};

type Drawable = HTMLImageElement | HTMLVideoElement | ImageBitmap;

function render(source: Drawable, width: number, height: number, maxWidth: number) {
  const scale = Math.min(1, maxWidth / width);
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(width * scale));
  canvas.height = Math.max(1, Math.round(height * scale));
  const context = canvas.getContext("2d");
  if (!context) throw new Error("This image could not be opened.");
  context.imageSmoothingQuality = "high";
  context.drawImage(source, 0, 0, canvas.width, canvas.height);
  return canvas;
}

function toJpeg(canvas: HTMLCanvasElement, quality: number): Promise<Blob> {
  return new Promise((resolve, reject) =>
    canvas.toBlob(
      (blob) => (blob ? resolve(blob) : reject(new Error("This image could not be opened."))),
      "image/jpeg",
      quality,
    ),
  );
}

function averageColor(canvas: HTMLCanvasElement): string {
  const dot = document.createElement("canvas");
  dot.width = dot.height = 1;
  const context = dot.getContext("2d", { willReadFrequently: true });
  if (!context) return "#262624";
  context.drawImage(canvas, 0, 0, 1, 1);
  const [r, g, b] = context.getImageData(0, 0, 1, 1).data;
  return `#${[r, g, b].map((channel) => (channel ?? 0).toString(16).padStart(2, "0")).join("")}`;
}

function loadImage(file: Blob): Promise<{ source: Drawable; done: () => void }> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const image = new Image();
    image.onload = () => resolve({ source: image, done: () => URL.revokeObjectURL(url) });
    image.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error("This image could not be opened."));
    };
    image.src = url;
  });
}

async function prepareImage(file: File): Promise<Prepared> {
  if (file.size > MAX_IMAGE_BYTES) throw new Error("This image is larger than 25 MB.");
  const { source, done } = await loadImage(file);
  try {
    const image = source as HTMLImageElement;
    const width = image.naturalWidth;
    const height = image.naturalHeight;
    if (!width || !height) throw new Error("This image could not be opened.");
    if (width * height > MAX_PIXELS) throw new Error("This image is larger than 60 megapixels.");
    const canvas = render(image, width, height, THUMB_WIDTH);
    return {
      kind: "image",
      width,
      height,
      extension: EXTENSIONS[file.type]!,
      thumb: await toJpeg(canvas, 0.82),
      color: averageColor(canvas),
    };
  } finally {
    done();
  }
}

function loadVideoFrame(file: Blob): Promise<{ video: HTMLVideoElement; done: () => void }> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const video = document.createElement("video");
    const done = () => {
      video.removeAttribute("src");
      video.load();
      URL.revokeObjectURL(url);
    };
    const fail = () => {
      done();
      reject(new Error("This video could not be opened."));
    };
    const timeout = setTimeout(fail, 20_000);
    video.muted = true;
    video.playsInline = true;
    video.preload = "auto";
    video.onerror = () => {
      clearTimeout(timeout);
      fail();
    };
    video.onloadeddata = () => {
      // A moment in, so the still is not a black fade-in frame.
      const target = Math.min(1, (video.duration || 2) / 2);
      video.onseeked = () => {
        clearTimeout(timeout);
        resolve({ video, done });
      };
      video.currentTime = target;
    };
    video.src = url;
  });
}

async function prepareVideo(file: File): Promise<Prepared> {
  if (file.size > MAX_VIDEO_BYTES) throw new Error("This video is larger than 200 MB.");
  const { video, done } = await loadVideoFrame(file);
  try {
    const width = video.videoWidth;
    const height = video.videoHeight;
    if (!width || !height) throw new Error("This video could not be opened.");
    const thumbCanvas = render(video, width, height, THUMB_WIDTH);
    return {
      kind: "video",
      width,
      height,
      extension: EXTENSIONS[file.type]!,
      thumb: await toJpeg(thumbCanvas, 0.82),
      poster: await toJpeg(render(video, width, height, POSTER_WIDTH), 0.86),
      color: averageColor(thumbCanvas),
    };
  } finally {
    done();
  }
}

/** Validate one added file and build its previews. Throws a sentence the person can act on. */
export function prepareFile(file: File): Promise<Prepared> {
  if (IMAGE_TYPES.includes(file.type)) return prepareImage(file);
  if (VIDEO_TYPES.includes(file.type)) return prepareVideo(file);
  return Promise.reject(
    new Error("Use a JPEG, PNG or WebP image, or an MP4, MOV or WebM video."),
  );
}

/** A grid thumbnail for a picture the app already has (e.g. a bundled sample). */
export async function thumbnailOf(blob: Blob): Promise<{ thumb: Blob; color: string }> {
  const { source, done } = await loadImage(blob);
  try {
    const image = source as HTMLImageElement;
    const canvas = render(image, image.naturalWidth, image.naturalHeight, THUMB_WIDTH);
    return { thumb: await toJpeg(canvas, 0.82), color: averageColor(canvas) };
  } finally {
    done();
  }
}

export function titleFromFileName(name: string): string {
  const base = name.replace(/\.[^.]+$/, "").replace(/[_-]+/g, " ").trim();
  return base ? base.charAt(0).toUpperCase() + base.slice(1) : "Untitled";
}
