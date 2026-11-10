import { test } from "node:test";
import assert from "node:assert/strict";
process.env.TZ = "America/New_York";
const { nextBoundary, nextChange, toSchedule, clampCustomMinutes, describeWait } = await import(
  "../src/lib/schedule.ts"
);

const hourly = { type: "interval", minutes: 60 };
const threeHourly = { type: "interval", minutes: 180 };
const active = { schedule: hourly, paused: false, itemCount: 3 };

test("interval changes align to local clock boundaries", () => {
  assert.equal(
    nextBoundary(hourly, new Date("2026-11-04T12:30:00Z")).toISOString(),
    "2026-11-04T13:00:00.000Z",
  );
  assert.equal(
    nextBoundary(hourly, new Date("2026-11-04T14:00:00Z")).toISOString(),
    "2026-11-04T15:00:00.000Z",
  );
  assert.equal(
    nextBoundary(threeHourly, new Date("2026-11-04T15:01:00Z")).toISOString(),
    "2026-11-04T17:00:00.000Z",
  );
});

test("daily time rolls forward at the boundary and across month end", () => {
  const next = nextBoundary({ type: "daily", time: "07:00" }, new Date(2026, 10, 30, 7, 0));
  assert.equal(next.getMonth(), 11);
  assert.equal(next.getDate(), 1);
  assert.equal(next.getHours(), 7);
});

test("daily timing keeps the local hour across daylight saving changes", () => {
  const next = nextBoundary({ type: "daily", time: "07:00" }, new Date(2027, 2, 13, 8, 0));
  assert.equal(next.getDate(), 14);
  assert.equal(next.getHours(), 7);
  assert.equal(next.getTimezoneOffset(), 240);
});

test("paused or empty rotations have no next change", () => {
  assert.equal(nextChange({ ...active, paused: true }), null);
  assert.equal(nextChange({ ...active, itemCount: 0 }), null);
  assert.ok(nextChange(active) instanceof Date);
});

test("invalid timing cannot produce an invalid date", () => {
  for (const schedule of [
    { type: "weekly" },
    { type: "daily", time: "25:00" },
    { type: "interval", minutes: 0 },
    { type: "interval", minutes: 1.5 },
    { type: "interval", minutes: 5000 },
  ])
    assert.equal(nextBoundary(schedule), null);
});

test("starting at an arbitrary minute does not shift the clock schedule", () => {
  const next = nextBoundary(hourly, new Date(2026, 10, 4, 15, 52, 37, 123));
  assert.equal(next.getHours(), 16);
  assert.equal(next.getMinutes(), 0);
  assert.equal(next.getSeconds(), 0);
  assert.equal(next.getMilliseconds(), 0);
  const midnight = nextBoundary(threeHourly, new Date(2026, 10, 4, 23, 52));
  assert.equal(midnight.getDate(), 5);
  assert.equal(midnight.getHours(), 0);
  assert.equal(midnight.getMinutes(), 0);
});

test("short intervals land on quarter and half hours", () => {
  const quarter = nextBoundary({ type: "interval", minutes: 15 }, new Date(2026, 10, 4, 9, 16));
  assert.equal(quarter.getHours(), 9);
  assert.equal(quarter.getMinutes(), 30);
  const half = nextBoundary({ type: "interval", minutes: 30 }, new Date(2026, 10, 4, 9, 30));
  assert.equal(half.getHours(), 10);
  assert.equal(half.getMinutes(), 0);
});

test("an interval that does not divide the day restarts at midnight", () => {
  // 50 minutes: …, 22:30, 23:20, then midnight rather than 00:10.
  const late = nextBoundary({ type: "interval", minutes: 50 }, new Date(2026, 10, 4, 23, 25));
  assert.equal(late.getDate(), 5);
  assert.equal(late.getHours(), 0);
  assert.equal(late.getMinutes(), 0);
  const first = nextBoundary({ type: "interval", minutes: 50 }, new Date(2026, 10, 5, 0, 0));
  assert.equal(first.getHours(), 0);
  assert.equal(first.getMinutes(), 50);
});

test("an interval survives the skipped daylight-saving hour", () => {
  // 2027-03-14 02:00 does not exist in New York; the result must still be in the future.
  const now = new Date(2027, 2, 14, 1, 59, 30);
  const next = nextBoundary(hourly, now);
  assert.ok(next > now);
  assert.ok(next - now <= 60 * 60_000 + 1000);
});

test("a short interval stays ahead of the clock in the repeated daylight-saving hour", () => {
  // 2026-11-01 in New York: 01:00-01:59 happens twice. 06:10Z is 01:10 the second time.
  const now = new Date("2026-11-01T06:10:00Z");
  const next = nextBoundary({ type: "interval", minutes: 15 }, now);
  assert.equal(next.toISOString(), "2026-11-01T06:15:00.000Z");
  // And the first time round (05:10Z is 01:10 before the clocks go back).
  const earlier = nextBoundary({ type: "interval", minutes: 15 }, new Date("2026-11-01T05:10:00Z"));
  assert.equal(earlier.toISOString(), "2026-11-01T05:15:00.000Z");
});

test("presets map to schedules and custom lengths are clamped", () => {
  const base = { customMinutes: 45, dailyTime: "08:15" };
  assert.deepEqual(toSchedule({ ...base, interval: "15m" }), { type: "interval", minutes: 15 });
  assert.deepEqual(toSchedule({ ...base, interval: "three" }), { type: "interval", minutes: 180 });
  assert.deepEqual(toSchedule({ ...base, interval: "day" }), { type: "daily", time: "08:15" });
  assert.deepEqual(toSchedule({ ...base, interval: "custom" }), { type: "interval", minutes: 45 });
  assert.equal(clampCustomMinutes(1), 5);
  assert.equal(clampCustomMinutes(99999), 1440);
  assert.equal(clampCustomMinutes(Number.NaN), 60);
});

test("the wait is described in plain units", () => {
  const now = new Date(2026, 10, 4, 9, 0);
  assert.equal(describeWait(new Date(2026, 10, 4, 9, 12), now), "in 12 min");
  assert.equal(describeWait(new Date(2026, 10, 4, 11, 0), now), "in 2 hr");
  assert.equal(describeWait(new Date(2026, 10, 4, 11, 5), now), "in 2 hr 5 min");
});
