import { test } from "node:test";
import assert from "node:assert/strict";
import { nextChange } from "../src/schedule.js";
process.env.TZ = "America/New_York";
const base = {
  frequency: "hour",
  collections: [{ enabled: true, images: [{}] }],
};
test("interval preview aligns to local clock boundaries", () => {
  assert.equal(
    nextChange(base, new Date("2026-11-04T12:30:00Z")).toISOString(),
    "2026-11-04T13:00:00.000Z",
  );
  assert.equal(
    nextChange(base, new Date("2026-11-04T14:00:00Z")).toISOString(),
    "2026-11-04T15:00:00.000Z",
  );
  assert.equal(
    nextChange(
      { ...base, frequency: "three" },
      new Date("2026-11-04T15:01:00Z"),
    ).toISOString(),
    "2026-11-04T17:00:00.000Z",
  );
});
test("daily time rolls forward at the boundary and across month end", () => {
  const now = new Date(2026, 10, 30, 7, 0);
  const next = nextChange(
    { ...base, frequency: "day", dailyTime: "07:00" },
    now,
  );
  assert.equal(next.getMonth(), 11);
  assert.equal(next.getDate(), 1);
  assert.equal(next.getHours(), 7);
});
test("daily timing keeps local hour across daylight saving changes", () => {
  const next = nextChange(
    { ...base, frequency: "day", dailyTime: "07:00" },
    new Date(2027, 2, 13, 8, 0),
  );
  assert.equal(next.getDate(), 14);
  assert.equal(next.getHours(), 7);
  assert.equal(next.getTimezoneOffset(), 240);
});
test("pause, empty or disconnected collections have no next change", () => {
  for (const patch of [
    { paused: true },
    { collections: [] },
    { collections: [{ enabled: true, images: [] }] },
    { collections: [{ enabled: false, images: [{}] }] },
  ])
    assert.equal(nextChange({ ...base, ...patch }), null);
});
test("invalid timing configuration cannot produce an invalid date", () => {
  for (const patch of [
    { frequency: "week" },
    { frequency: "day", dailyTime: "25:00" },
  ])
    assert.equal(nextChange({ ...base, ...patch }), null);
});

test("starting at an arbitrary minute does not shift the clock schedule", () => {
  const now = new Date(2026, 10, 4, 15, 52, 37, 123);
  const next = nextChange(base, now);
  assert.equal(next.getHours(), 16);
  assert.equal(next.getMinutes(), 0);
  assert.equal(next.getSeconds(), 0);
  assert.equal(next.getMilliseconds(), 0);
  const midnight = nextChange({...base, frequency: "three"}, new Date(2026, 10, 4, 23, 52));
  assert.equal(midnight.getDate(), 5);
  assert.equal(midnight.getHours(), 0);
  assert.equal(midnight.getMinutes(), 0);
});
