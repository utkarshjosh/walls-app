import { test } from "node:test";
import assert from "node:assert/strict";
import {
  emptyEngine,
  advance,
  advanceSlots,
  back,
  show,
  reconcile,
  HISTORY_LIMIT,
} from "../src/lib/rotation.ts";

const ids = ["a", "b", "c", "d", "e"];

// Small deterministic generator so shuffle tests are repeatable.
function seeded(seed) {
  let s = seed;
  return () => {
    s = (s * 1664525 + 1013904223) % 4294967296;
    return s / 4294967296;
  };
}

test("shuffle shows every wallpaper once before any repeats", () => {
  let state = emptyEngine();
  const random = seeded(7);
  const seen = [];
  for (let i = 0; i < ids.length; i++) {
    state = advance(state, ids, { shuffle: true, random });
    seen.push(state.current[0]);
  }
  assert.deepEqual([...seen].sort(), ids);
});

test("a new shuffle round never opens with the wallpaper on screen", () => {
  for (let seed = 1; seed <= 40; seed++) {
    let state = emptyEngine();
    const random = seeded(seed);
    let previous;
    for (let i = 0; i < ids.length * 4; i++) {
      state = advance(state, ids, { shuffle: true, random });
      assert.notEqual(state.current[0], previous, `seed ${seed} repeated at step ${i}`);
      previous = state.current[0];
    }
  }
});

test("in order walks the list and wraps", () => {
  let state = emptyEngine();
  const shown = [];
  for (let i = 0; i < 7; i++) {
    state = advance(state, ids, { shuffle: false });
    shown.push(state.current[0]);
  }
  assert.deepEqual(shown, ["a", "b", "c", "d", "e", "a", "b"]);
});

test("previous steps back through what was actually shown", () => {
  let state = emptyEngine();
  const random = seeded(3);
  const shown = [];
  for (let i = 0; i < 4; i++) {
    state = advance(state, ids, { shuffle: true, random });
    shown.push(state.current[0]);
  }
  state = back(state);
  assert.equal(state.current[0], shown[2]);
  state = back(state);
  assert.equal(state.current[0], shown[1]);
  state = back(state);
  assert.equal(state.current[0], shown[0]);
  assert.equal(back(state), null);
});

test("picking a wallpaper by hand is remembered in history", () => {
  let state = advance(emptyEngine(), ids, { shuffle: false });
  state = show(state, ["d"]);
  assert.deepEqual(state.current, ["d"]);
  assert.deepEqual(back(state).current, ["a"]);
  // In order continues from the hand-picked one.
  assert.deepEqual(advance(state, ids, { shuffle: false }).current, ["e"]);
});

test("removed wallpapers are forgotten everywhere", () => {
  let state = emptyEngine();
  for (let i = 0; i < 3; i++) state = advance(state, ids, { shuffle: false });
  state = reconcile(state, ["a", "c", "e"]);
  assert.deepEqual(state.current, ["c"]);
  assert.deepEqual(state.history, [["a"]]);
});

test("each display gets a different wallpaper when there are enough", () => {
  let state = emptyEngine();
  const random = seeded(11);
  for (let i = 0; i < 12; i++) {
    state = advance(state, ids, { shuffle: true, count: 3, random });
    assert.equal(new Set(state.current).size, 3);
  }
  const ordered = advance(emptyEngine(), ids, { shuffle: false, count: 2 });
  assert.deepEqual(ordered.current, ["a", "b"]);
  // Fewer wallpapers than displays: repeat rather than stall.
  const scarce = advance(emptyEngine(), ["a"], { shuffle: true, count: 2, random });
  assert.deepEqual(scarce.current, ["a", "a"]);
});

test("nothing to show leaves the state alone and history is capped", () => {
  const empty = emptyEngine();
  assert.equal(advance(empty, [], { shuffle: true }), empty);
  let state = emptyEngine();
  for (let i = 0; i < HISTORY_LIMIT + 20; i++) state = advance(state, ids, { shuffle: false });
  assert.equal(state.history.length, HISTORY_LIMIT);
});

test("a per-display frame keeps its positions when a wallpaper is removed", () => {
  const state = { bag: [], current: ["a", "b"], history: [["c", "d"], ["b", "b"]] };
  const after = reconcile(state, ["a", "c", "d"]);
  assert.deepEqual(after.current, ["a", ""]);
  // A frame with nothing left is dropped; one with something left keeps its shape.
  assert.deepEqual(after.history, [["c", "d"]]);
  assert.deepEqual(reconcile(state, ["c"]).current, []);
});

test("each display only gets wallpapers that are allowed on it", () => {
  // Display 0 is demanding (only a and b are large enough); display 1 takes anything.
  const allowed = [new Set(["a", "b"]), new Set(ids)];
  for (const shuffle of [true, false]) {
    let state = emptyEngine();
    const random = seeded(5);
    for (let i = 0; i < 20; i++) {
      state = advanceSlots(state, ids, allowed, { shuffle, random });
      assert.ok(allowed[0].has(state.current[0]), `display 0 got ${state.current[0]}`);
      assert.ok(allowed[1].has(state.current[1]));
      assert.notEqual(state.current[0], state.current[1]);
    }
  }
});

test("a display with nothing large enough is left alone", () => {
  const allowed = [new Set(), new Set(["c", "d"])];
  let state = advanceSlots(emptyEngine(), ids, allowed, { shuffle: false });
  assert.deepEqual(state.current, ["", "c"]);
  state = advanceSlots(state, ids, allowed, { shuffle: false });
  assert.deepEqual(state.current, ["", "d"]);
  state = advanceSlots(state, ids, allowed, { shuffle: false });
  assert.deepEqual(state.current, ["", "c"]);
  // Nothing allowed anywhere: nothing happens.
  const idle = emptyEngine();
  assert.equal(advanceSlots(idle, ids, [new Set(), new Set()], { shuffle: true }), idle);
});

test("with everything allowed, per-display frames match the plain rules", () => {
  const everything = [new Set(ids), new Set(ids)];
  let state = emptyEngine();
  const shown = [];
  for (let i = 0; i < 3; i++) {
    state = advanceSlots(state, ids, everything, { shuffle: false });
    shown.push(state.current);
  }
  assert.deepEqual(shown, [["a", "b"], ["c", "d"], ["e", "a"]]);
  // Shuffled: every wallpaper appears before any repeats.
  state = emptyEngine();
  const random = seeded(9);
  const seen = [];
  for (let i = 0; i < 2; i++) {
    state = advanceSlots(state, ids, everything, { shuffle: true, random });
    seen.push(...state.current);
  }
  assert.equal(new Set(seen).size, 4);
});

test("a display with one allowed wallpaper shares it rather than going empty", () => {
  const allowed = [new Set(["a"]), new Set(["a"])];
  const state = advanceSlots(emptyEngine(), ids, allowed, { shuffle: true, random: seeded(2) });
  assert.deepEqual(state.current, ["a", "a"]);
});

test("in order, a demanding first display cannot freeze the rotation", () => {
  // The real desk: the MacBook comes first and only takes the two largest
  // pictures; the monitor takes those and a smaller one.
  const plan = ["pano", "fourk", "sample", "hd"];
  const allowed = [new Set(["pano", "fourk"]), new Set(["pano", "fourk", "sample"])];
  let state = emptyEngine();
  const frames = [];
  for (let i = 0; i < 6; i++) {
    state = advanceSlots(state, plan, allowed, { shuffle: false });
    frames.push(state.current.join("+"));
  }
  assert.deepEqual(frames, [
    "pano+fourk",
    "fourk+sample",
    "pano+fourk",
    "fourk+sample",
    "pano+fourk",
    "fourk+sample",
  ]);
  // Every display with a choice shows something new each time.
  for (let i = 1; i < frames.length; i++) {
    const [before0, before1] = frames[i - 1].split("+");
    const [after0, after1] = frames[i].split("+");
    assert.notEqual(after0, before0);
    assert.notEqual(after1, before1);
  }
});
