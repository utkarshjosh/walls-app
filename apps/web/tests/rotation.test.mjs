import { test } from "node:test";
import assert from "node:assert/strict";
import { emptyEngine, advance, back, show, reconcile, HISTORY_LIMIT } from "../src/lib/rotation.ts";

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
