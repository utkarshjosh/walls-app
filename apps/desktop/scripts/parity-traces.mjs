// Parity check, part 1 of 2.
//
// The rotation and size rules exist twice: in TypeScript (apps/web/src/lib/
// rotation.ts and fit.ts) and in Rust (src-tauri/src/engine.rs and fit.rs).
// This script runs the TypeScript over a few thousand generated cases and
// writes every result to a JSON file; the Rust test in src-tauri/src/parity.rs
// replays the same cases and must get the same answers.
//
//   node --experimental-strip-types scripts/parity-traces.mjs ../web /tmp/still-traces.json
//   cd src-tauri && STILL_PARITY_TRACES=/tmp/still-traces.json cargo test parity -- --nocapture
//
// It only reads apps/web. Run it after changing either copy of the rules.
import { writeFileSync } from "node:fs";
import { resolve } from "node:path";
const web = resolve(process.argv[2]);
const engine = await import(`${web}/src/lib/rotation.ts`);
const fit = await import(`${web}/src/lib/fit.ts`);

function seeded(seed) {
  let s = seed;
  return () => {
    s = (s * 1664525 + 1013904223) % 4294967296;
    return s / 4294967296;
  };
}
const pick = (random, list) => list[Math.floor(random() * list.length)];
const subset = (random, list, chance) => list.filter(() => random() < chance);

const scenarios = [];
for (let n = 0; n < 400; n++) {
  const plan = seeded(1000 + n); // decides what the scenario does
  const poolSize = 1 + Math.floor(plan() * 7);
  const ids = ["a", "b", "c", "d", "e", "f", "g"].slice(0, poolSize);
  const shuffle = plan() < 0.6;
  const displays = 1 + Math.floor(plan() * 3);
  const seed = 1 + Math.floor(plan() * 100000);
  const random = seeded(seed); // what the engine draws with
  let state = engine.emptyEngine();
  const ops = [];
  for (let step = 0; step < 30; step++) {
    const roll = plan();
    let op;
    if (roll < 0.35) {
      const count = 1 + Math.floor(plan() * displays);
      op = { kind: "advance", count };
      state = engine.advance(state, ids, { shuffle, count, random });
    } else if (roll < 0.7) {
      const allowed = Array.from({ length: displays }, () => subset(plan, ids, plan() < 0.15 ? 0 : 0.6));
      op = { kind: "advanceSlots", allowed };
      state = engine.advanceSlots(state, ids, allowed.map((list) => new Set(list)), { shuffle, random });
    } else if (roll < 0.8) {
      op = { kind: "back" };
      state = engine.back(state) ?? state;
    } else if (roll < 0.9) {
      const frame = Array.from({ length: 1 + Math.floor(plan() * displays) }, () => (plan() < 0.2 ? "" : pick(plan, ids)));
      op = { kind: "show", frame };
      state = engine.show(state, frame);
    } else {
      const alive = subset(plan, ids, 0.75);
      op = { kind: "reconcile", alive };
      state = engine.reconcile(state, alive);
    }
    ops.push({ ...op, after: JSON.parse(JSON.stringify(state)) });
  }
  scenarios.push({ ids, shuffle, seed, ops });
}

const fits = [];
const sizes = [0, 640, 941, 1080, 1280, 1672, 1920, 2400, 2560, 2880, 3840, 5120, 7680];
for (let n = 0; n < 3000; n++) {
  const r = seeded(50000 + n);
  const item = { kind: pick(r, ["image", "image", "video", "scene"]), width: pick(r, sizes), height: pick(r, sizes) };
  // A scene that draws itself whole on each display; the key is left out
  // otherwise, as it is in real data.
  if (r() < 0.25) item.perDisplay = r() < 0.8;
  const displays = Array.from({ length: Math.floor(r() * 4) }, () => ({
    x: Math.floor(r() * 6000) - 3000,
    y: Math.floor(r() * 3000) - 1500,
    width: pick(r, [1280, 1440, 1512, 1728, 1920, 2560, 3008]),
    height: pick(r, [800, 900, 982, 1080, 1117, 1440, 1692]),
    scale: pick(r, [1, 1, 2, 1.5]),
  }));
  const one = displays[0];
  fits.push({
    item,
    displays,
    enlargementOn: one && item.width > 0 && item.height > 0 ? fit.enlargementOn(item, one) : null,
    enlargementSpanned: item.width > 0 && item.height > 0 ? fit.enlargementSpanned(item, displays) : null,
    fitsDisplay: one ? fit.fitsDisplay(item, one) : null,
    fitsEvery: fit.fitsEvery(item, displays),
    fitsAny: fit.fitsAny(item, displays),
    canSpan: fit.canSpan(item, displays),
    placementSame: fit.placementFor(item, displays, "same"),
    placementSpan: fit.placementFor(item, displays, "span"),
  });
}
writeFileSync(process.argv[3], JSON.stringify({ scenarios, fits }));
console.log(`wrote ${scenarios.length} engine scenarios (${scenarios.length * 30} steps) and ${fits.length} size cases`);
