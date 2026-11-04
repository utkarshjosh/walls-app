// Local timing preview. The future device manifest remains authoritative on Mac.
export function nextChange(
  { frequency, dailyTime = "07:00", paused, collections },
  now = new Date(),
) {
  if (paused || !collections.some((c) => c.enabled && c.images.length))
    return null;
  if (frequency === "day") {
    if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(dailyTime)) return null;
    const [hour, minute] = dailyTime.split(":").map(Number);
    const next = new Date(now);
    next.setHours(hour, minute, 0, 0);
    if (next <= now) {
      next.setDate(next.getDate() + 1);
      next.setHours(hour, minute, 0, 0);
    }
    return next;
  }
  const hours = { hour: 1, three: 3 }[frequency];
  if (!hours) return null;
  const next = new Date(now);
  next.setHours(now.getHours() + hours - (now.getHours() % hours), 0, 0, 0);
  return next;
}
