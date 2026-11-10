// When the next automatic wallpaper change happens.
//
// The Mac app's native core applies the same rules (apps/desktop/src-tauri);
// this copy drives the in-app "next change" time and the browser preview.
// Pure and dependency-free so `node --test` can import it directly.

export type Schedule =
  | { type: "interval"; minutes: number }
  | { type: "daily"; time: string };

export type IntervalPreset = "15m" | "30m" | "hour" | "three" | "day" | "custom";

const DAILY = /^([01]\d|2[0-3]):[0-5]\d$/;
const MINUTES_PER_DAY = 1440;

export const MIN_CUSTOM_MINUTES = 5;
export const MAX_CUSTOM_MINUTES = MINUTES_PER_DAY;

export function toSchedule(settings: {
  interval: IntervalPreset;
  customMinutes: number;
  dailyTime: string;
}): Schedule {
  switch (settings.interval) {
    case "15m":
      return { type: "interval", minutes: 15 };
    case "30m":
      return { type: "interval", minutes: 30 };
    case "hour":
      return { type: "interval", minutes: 60 };
    case "three":
      return { type: "interval", minutes: 180 };
    case "day":
      return { type: "daily", time: settings.dailyTime };
    case "custom":
      return { type: "interval", minutes: clampCustomMinutes(settings.customMinutes) };
  }
}

export function clampCustomMinutes(value: number): number {
  if (!Number.isFinite(value)) return 60;
  return Math.min(MAX_CUSTOM_MINUTES, Math.max(MIN_CUSTOM_MINUTES, Math.round(value)));
}

/**
 * The next moment a change is due, strictly after `now`, or null for an
 * invalid schedule.
 *
 * Intervals land on local clock boundaries counted from local midnight, so
 * "every hour" means every :00 and "every three hours" means 00:00, 03:00, …
 * regardless of when the app was opened. The count restarts at midnight, which
 * only matters for lengths that do not divide a day evenly.
 */
export function nextBoundary(schedule: Schedule, now: Date = new Date()): Date | null {
  if (schedule.type === "daily") {
    if (!DAILY.test(schedule.time)) return null;
    const [hour, minute] = schedule.time.split(":").map(Number) as [number, number];
    const next = new Date(now);
    next.setHours(hour, minute, 0, 0);
    if (next <= now) {
      next.setDate(next.getDate() + 1);
      // Set the time again: crossing a daylight-saving change can shift it.
      next.setHours(hour, minute, 0, 0);
    }
    return next;
  }

  if (schedule.type !== "interval") return null;
  const step = schedule.minutes;
  if (!Number.isInteger(step) || step < 1 || step > MINUTES_PER_DAY) return null;

  // Wall-clock minutes since local midnight, so daylight-saving days still
  // produce boundaries on the clock face rather than on elapsed time.
  const sinceMidnight = now.getHours() * 60 + now.getMinutes();
  const target = (Math.floor(sinceMidnight / step) + 1) * step;
  const next = new Date(now);
  if (target >= MINUTES_PER_DAY) {
    next.setDate(next.getDate() + 1);
    next.setHours(0, 0, 0, 0);
  } else {
    next.setHours(Math.floor(target / 60), target % 60, 0, 0);
  }
  // Daylight-saving changes can put the clock-face answer at or before `now`:
  // a skipped hour normalises backwards, and in the repeated hour the same
  // clock time names the earlier of two moments. Step forward until it is ahead.
  let due = next.getTime();
  while (due <= now.getTime()) due += step * 60_000;
  return new Date(due);
}

/** Next change for a whole rotation: nothing is due while paused or with nothing to show. */
export function nextChange(
  input: { schedule: Schedule; paused: boolean; itemCount: number },
  now: Date = new Date(),
): Date | null {
  if (input.paused || input.itemCount < 1) return null;
  return nextBoundary(input.schedule, now);
}

/** "in 12 min", "in 2 hr 5 min" — for the single next-change line. */
export function describeWait(target: Date, now: Date = new Date()): string {
  const minutes = Math.max(1, Math.ceil((target.getTime() - now.getTime()) / 60_000));
  if (minutes < 60) return `in ${minutes} min`;
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return rest ? `in ${hours} hr ${rest} min` : `in ${hours} hr`;
}
