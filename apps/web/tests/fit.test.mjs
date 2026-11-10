import { test } from "node:test";
import assert from "node:assert/strict";
import {
  SHARP_LIMIT,
  canSpan,
  enlargementOn,
  enlargementSpanned,
  fitsAny,
  fitsDisplay,
  fitsEvery,
  placementFor,
  rotationUse,
} from "../src/lib/fit.ts";

// A real desk: a 16-inch MacBook Pro beside a 24-inch 1080p monitor.
const macbook = { x: -1728, y: 0, width: 1728, height: 1117, scale: 2 }; // 3456 x 2234 pixels
const monitor = { x: 0, y: 0, width: 1920, height: 1080, scale: 1 };
const desk = [macbook, monitor];

const image = (width, height) => ({ kind: "image", width, height });
const fourK = image(3840, 2400);
const panorama = image(7680, 2400);
const sample = image(1672, 941);
const hd720 = image(1280, 720);

const near = (actual, expected) => assert.ok(Math.abs(actual - expected) < 0.005, `${actual} vs ${expected}`);

test("enlargement is measured against the display's real pixels", () => {
  near(enlargementOn(fourK, macbook), 0.931); // 2234 / 2400
  near(enlargementOn(sample, macbook), 2.374); // 2234 / 941
  near(enlargementOn(sample, monitor), 1.148); // 1920 / 1672
  near(enlargementOn(hd720, monitor), 1.5);
});

test("a picture fits a display when it is enlarged by a quarter at most", () => {
  assert.equal(SHARP_LIMIT, 1.25);
  assert.ok(fitsDisplay(fourK, macbook));
  assert.ok(fitsDisplay(fourK, monitor));
  assert.ok(!fitsDisplay(sample, macbook));
  assert.ok(fitsDisplay(sample, monitor));
  assert.ok(!fitsDisplay(hd720, monitor));
  assert.ok(fitsDisplay(image(2880, 1800), macbook)); // 1.24
  assert.ok(!fitsDisplay(image(2560, 1440), macbook)); // 1.55
});

test("fitting every display and fitting any display are different questions", () => {
  assert.ok(fitsEvery(fourK, desk));
  assert.ok(!fitsEvery(sample, desk));
  assert.ok(fitsAny(sample, desk));
  assert.ok(!fitsAny(hd720, desk));
  assert.ok(fitsAny(hd720, []));
});

test("stretching across displays is judged on the densest display", () => {
  // Canvas 3648 x 1117 points. The panorama gives 0.475 points per pixel: 0.95 on the Retina side.
  near(enlargementSpanned(panorama, desk), 0.95);
  // A 4K picture gives 0.95 points per pixel: 1.9 on the Retina side.
  near(enlargementSpanned(fourK, desk), 1.9);
  assert.ok(canSpan(panorama, desk));
  assert.ok(!canSpan(fourK, desk));
  assert.ok(!canSpan(sample, desk));
});

test("two ordinary monitors need far fewer pixels to span", () => {
  const pair = [monitor, { ...monitor, x: 1920 }];
  near(enlargementSpanned(fourK, pair), 1.0); // 3840 points over 3840 pixels
  assert.ok(canSpan(fourK, pair));
  assert.ok(!canSpan(image(1920, 1080), pair));
});

test("displays stacked or offset use the whole bounding box", () => {
  const stacked = [monitor, { ...monitor, x: 200, y: 1080 }];
  // Box 2120 x 2160 points; a 3840 x 2400 picture covers it at 0.9 points per pixel.
  near(enlargementSpanned(fourK, stacked), 0.9);
});

test("one display can never be spanned", () => {
  assert.ok(!canSpan(panorama, [macbook]));
  assert.ok(!canSpan({ kind: "scene", width: 0, height: 0 }, [macbook]));
});

test("rotation spans what it can, shows the rest on each display, and skips what is too small", () => {
  assert.equal(placementFor(panorama, desk, "span"), "span");
  assert.equal(placementFor(fourK, desk, "span"), "same");
  assert.equal(placementFor(sample, desk, "span"), "skip");
  assert.equal(placementFor(panorama, desk, "same"), "same");
  assert.equal(placementFor(sample, desk, "same"), "skip");
  assert.equal(placementFor(sample, [monitor], "same"), "same");
});

test("video is allowed more enlargement than a still picture", () => {
  const video = (width, height) => ({ kind: "video", width, height });
  assert.ok(fitsDisplay(video(1920, 1080), macbook)); // 2.07
  assert.ok(!fitsDisplay(video(1280, 720), macbook)); // 3.1
  assert.ok(fitsDisplay(video(1280, 720), monitor)); // 1.5
});

test("scenes and pictures of unknown size always pass", () => {
  const scene = { kind: "scene", width: 1920, height: 1200 };
  assert.ok(fitsEvery(scene, desk));
  assert.ok(canSpan(scene, desk));
  assert.equal(placementFor(scene, desk, "span"), "span");
  const unknown = image(0, 0);
  assert.ok(fitsEvery(unknown, desk));
  assert.ok(canSpan(unknown, desk));
});

test("the rotation summary counts each outcome", () => {
  const library = [panorama, panorama, fourK, fourK, fourK, sample, hd720];
  assert.deepEqual(rotationUse(library, desk, "span"), { spanned: 2, shown: 3, skipped: 2 });
  assert.deepEqual(rotationUse(library, desk, "same"), { spanned: 0, shown: 5, skipped: 2 });
  // With a different wallpaper per display the sample is usable on the monitor.
  assert.deepEqual(rotationUse(library, desk, "each"), { spanned: 0, shown: 6, skipped: 1 });
});

test("a scene that draws itself whole on each display is never stretched", () => {
  const clock = { kind: "scene", width: 1920, height: 1200, perDisplay: true };
  assert.ok(fitsEvery(clock, desk));
  assert.ok(!canSpan(clock, desk));
  // Asking for "one across all" shows it complete on each display instead.
  assert.equal(placementFor(clock, desk, "span"), "same");
  assert.equal(placementFor(clock, desk, "same"), "same");
  assert.deepEqual(rotationUse([clock, panorama], desk, "span"), { spanned: 1, shown: 1, skipped: 0 });
  // An ordinary scene still spans.
  assert.ok(canSpan({ kind: "scene", width: 1920, height: 1200, perDisplay: false }, desk));
});
