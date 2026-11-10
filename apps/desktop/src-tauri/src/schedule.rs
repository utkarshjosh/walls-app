//! When the next automatic wallpaper change is due. Pure: the caller supplies
//! "now" in whatever time zone it likes, so tests do not depend on the machine.
//!
//! A port of `nextBoundary` in `apps/web/src/lib/schedule.ts`:
//!
//!  - Intervals land on local wall-clock boundaries counted from local midnight
//!    ("every hour" is every :00, "every three hours" is 00:00, 03:00, ...),
//!    never offset by when the app was opened. The count restarts at midnight,
//!    which only matters for lengths that do not divide a day evenly.
//!  - Daily changes happen at HH:MM local time.

use chrono::{DateTime, Duration, LocalResult, NaiveDateTime, Offset, TimeZone, Timelike};

use crate::model::Schedule;

const MINUTES_PER_DAY: u32 = 1440;

/// The next moment a change is due, strictly after `now`, or `None` for an
/// invalid schedule.
pub fn next_boundary<Tz: TimeZone>(schedule: &Schedule, now: &DateTime<Tz>) -> Option<DateTime<Tz>> {
    let zone = now.timezone();
    let local = now.naive_local();

    match schedule {
        Schedule::Daily { time } => {
            let (hour, minute) = parse_daily(time)?;
            let today = resolve_local(&zone, local.date().and_hms_opt(hour, minute, 0)?)?;
            if today > *now {
                return Some(today);
            }
            // Build tomorrow from the calendar date, not by adding 24 hours:
            // crossing a daylight-saving change would shift the clock time.
            resolve_local(&zone, local.date().succ_opt()?.and_hms_opt(hour, minute, 0)?)
        }
        Schedule::Interval { minutes } => {
            let step = whole_minutes(*minutes)?;
            // Wall-clock minutes since local midnight, so daylight-saving days
            // still produce boundaries on the clock face rather than on elapsed time.
            let since_midnight = local.hour() * 60 + local.minute();
            let target = (since_midnight / step + 1) * step;
            let wall = if target >= MINUTES_PER_DAY {
                local.date().succ_opt()?.and_hms_opt(0, 0, 0)?
            } else {
                local.date().and_hms_opt(target / 60, target % 60, 0)?
            };
            let mut next = resolve_local(&zone, wall)?;
            // A skipped daylight-saving hour can normalise onto or before `now`,
            // and in the repeated hour the wall time resolves to its first
            // occurrence, which may already be behind us. The TypeScript adds
            // one step here; looping keeps the "strictly after now" promise in
            // the repeated hour too, where a single step is not always enough.
            while next <= *now {
                next += Duration::minutes(i64::from(step));
            }
            Some(next)
        }
    }
}

/// Next change for a whole rotation: nothing is due while paused or with nothing to show.
pub fn next_change<Tz: TimeZone>(
    schedule: &Schedule,
    paused: bool,
    item_count: usize,
    now: &DateTime<Tz>,
) -> Option<DateTime<Tz>> {
    if paused || item_count < 1 {
        return None;
    }
    next_boundary(schedule, now)
}

/// "HH:MM", 24-hour, both parts two digits (the same shape the UI validates).
fn parse_daily(time: &str) -> Option<(u32, u32)> {
    let bytes = time.as_bytes();
    if bytes.len() != 5 || bytes[2] != b':' {
        return None;
    }
    let digit = |index: usize| -> Option<u32> {
        let byte = bytes[index];
        byte.is_ascii_digit().then(|| u32::from(byte - b'0'))
    };
    let hour = digit(0)? * 10 + digit(1)?;
    let minute = digit(3)? * 10 + digit(4)?;
    (hour <= 23 && minute <= 59).then_some((hour, minute))
}

fn whole_minutes(minutes: f64) -> Option<u32> {
    let valid = minutes.is_finite()
        && minutes.fract() == 0.0
        && (1.0..=f64::from(MINUTES_PER_DAY)).contains(&minutes);
    valid.then_some(minutes as u32)
}

/// Turn a local wall-clock time into an instant the way JavaScript's
/// `Date.setHours` does, so both sides agree around daylight-saving changes.
fn resolve_local<Tz: TimeZone>(zone: &Tz, wall: NaiveDateTime) -> Option<DateTime<Tz>> {
    match zone.from_local_datetime(&wall) {
        LocalResult::Single(instant) => Some(instant),
        // Clocks went back and this time happens twice: take the first.
        LocalResult::Ambiguous(first, _) => Some(first),
        // Clocks went forward and this time never happens: read it with the
        // offset from before the jump, which lands just as far past the gap.
        LocalResult::None => [3, 6, 12, 24, 48].into_iter().find_map(|hours_back| {
            let before = zone.from_local_datetime(&(wall - Duration::hours(hours_back))).latest()?;
            let offset_seconds = i64::from(before.offset().fix().local_minus_utc());
            Some(zone.from_utc_datetime(&(wall - Duration::seconds(offset_seconds))))
        }),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::{FixedOffset, NaiveDate, Utc};
    use chrono_tz::America::New_York;
    use chrono_tz::Tz;

    fn interval(minutes: f64) -> Schedule {
        Schedule::Interval { minutes }
    }

    fn daily(time: &str) -> Schedule {
        Schedule::Daily { time: time.to_string() }
    }

    fn wall(y: i32, mo: u32, d: u32, h: u32, mi: u32, s: u32) -> NaiveDateTime {
        NaiveDate::from_ymd_opt(y, mo, d).unwrap().and_hms_opt(h, mi, s).unwrap()
    }

    /// A zone with no daylight saving and an awkward offset, so a bug that
    /// mixes up UTC and local time cannot hide.
    fn kolkata() -> FixedOffset {
        FixedOffset::east_opt(5 * 3600 + 1800).unwrap()
    }

    fn at(zone: FixedOffset, y: i32, mo: u32, d: u32, h: u32, mi: u32, s: u32) -> DateTime<FixedOffset> {
        zone.from_local_datetime(&wall(y, mo, d, h, mi, s)).unwrap()
    }

    fn next_wall(schedule: &Schedule, now: DateTime<FixedOffset>) -> NaiveDateTime {
        next_boundary(schedule, &now).expect("schedule should be valid").naive_local()
    }

    #[test]
    fn hourly_from_an_arbitrary_minute_lands_on_the_next_hour() {
        for zone in [kolkata(), FixedOffset::west_opt(8 * 3600).unwrap(), FixedOffset::east_opt(0).unwrap()] {
            let now = at(zone, 2026, 11, 10, 13, 37, 21);
            assert_eq!(next_wall(&interval(60.0), now), wall(2026, 11, 10, 14, 0, 0));
        }
    }

    #[test]
    fn a_boundary_is_strictly_after_now() {
        let on_the_hour = at(kolkata(), 2026, 11, 10, 14, 0, 0);
        assert_eq!(next_wall(&interval(60.0), on_the_hour), wall(2026, 11, 10, 15, 0, 0));
        let just_before = on_the_hour - Duration::milliseconds(1);
        assert_eq!(next_wall(&interval(60.0), just_before), wall(2026, 11, 10, 14, 0, 0));
    }

    #[test]
    fn three_hourly_near_midnight_lands_on_midnight_next_day() {
        let now = at(kolkata(), 2026, 12, 31, 22, 10, 0);
        assert_eq!(next_wall(&interval(180.0), now), wall(2027, 1, 1, 0, 0, 0));
        let afternoon = at(kolkata(), 2026, 11, 10, 13, 37, 0);
        assert_eq!(next_wall(&interval(180.0), afternoon), wall(2026, 11, 10, 15, 0, 0));
    }

    #[test]
    fn fifteen_minutes_lands_on_quarter_hours() {
        let zone = kolkata();
        assert_eq!(next_wall(&interval(15.0), at(zone, 2026, 11, 10, 9, 7, 59)), wall(2026, 11, 10, 9, 15, 0));
        assert_eq!(next_wall(&interval(15.0), at(zone, 2026, 11, 10, 9, 45, 0)), wall(2026, 11, 10, 10, 0, 0));
        assert_eq!(next_wall(&interval(15.0), at(zone, 2026, 11, 10, 23, 52, 0)), wall(2026, 11, 11, 0, 0, 0));
    }

    #[test]
    fn a_non_dividing_interval_restarts_at_midnight() {
        let zone = kolkata();
        // Every 7 hours: 00:00, 07:00, 14:00, 21:00, then midnight again (not 04:00).
        let seven_hours = interval(420.0);
        assert_eq!(next_wall(&seven_hours, at(zone, 2026, 11, 10, 14, 30, 0)), wall(2026, 11, 10, 21, 0, 0));
        assert_eq!(next_wall(&seven_hours, at(zone, 2026, 11, 10, 21, 30, 0)), wall(2026, 11, 11, 0, 0, 0));
        assert_eq!(next_wall(&seven_hours, at(zone, 2026, 11, 11, 0, 0, 0)), wall(2026, 11, 11, 7, 0, 0));
        // Longer than half a day: the only boundaries are midnight and the step itself.
        assert_eq!(next_wall(&interval(1000.0), at(zone, 2026, 11, 10, 3, 0, 0)), wall(2026, 11, 10, 16, 40, 0));
        assert_eq!(next_wall(&interval(1000.0), at(zone, 2026, 11, 10, 17, 0, 0)), wall(2026, 11, 11, 0, 0, 0));
        // A whole day means every midnight.
        assert_eq!(next_wall(&interval(1440.0), at(zone, 2026, 11, 10, 0, 0, 0)), wall(2026, 11, 11, 0, 0, 0));
    }

    #[test]
    fn daily_rolls_to_tomorrow_at_the_boundary() {
        let zone = kolkata();
        let schedule = daily("09:30");
        assert_eq!(next_wall(&schedule, at(zone, 2026, 11, 10, 9, 29, 59)), wall(2026, 11, 10, 9, 30, 0));
        assert_eq!(next_wall(&schedule, at(zone, 2026, 11, 10, 9, 30, 0)), wall(2026, 11, 11, 9, 30, 0));
        assert_eq!(next_wall(&schedule, at(zone, 2026, 11, 10, 18, 0, 0)), wall(2026, 11, 11, 9, 30, 0));
        // Month and year ends roll over too.
        assert_eq!(next_wall(&daily("00:00"), at(zone, 2026, 12, 31, 0, 0, 0)), wall(2027, 1, 1, 0, 0, 0));
    }

    #[test]
    fn invalid_inputs_give_none() {
        let now = Utc.with_ymd_and_hms(2026, 11, 10, 12, 0, 0).unwrap();
        for minutes in [0.0, -5.0, 1441.0, 15.5, f64::NAN, f64::INFINITY] {
            assert!(next_boundary(&interval(minutes), &now).is_none(), "{minutes} minutes");
        }
        for time in ["", "9:30", "24:00", "09:60", "ab:cd", "09-30", "09:30:00", " 9:30", "0930"] {
            assert!(next_boundary(&daily(time), &now).is_none(), "daily at {time:?}");
        }
        assert!(next_boundary(&daily("23:59"), &now).is_some());
        assert!(next_boundary(&daily("00:00"), &now).is_some());
    }

    #[test]
    fn nothing_is_due_while_paused_or_empty() {
        let now = Utc.with_ymd_and_hms(2026, 11, 10, 12, 0, 0).unwrap();
        assert!(next_change(&interval(60.0), true, 5, &now).is_none());
        assert!(next_change(&interval(60.0), false, 0, &now).is_none());
        assert_eq!(
            next_change(&interval(60.0), false, 1, &now),
            Some(Utc.with_ymd_and_hms(2026, 11, 10, 13, 0, 0).unwrap())
        );
    }

    // Daylight saving, pinned to New York: clocks jump 02:00 -> 03:00 on
    // 8 March 2026 and fall back 02:00 -> 01:00 on 1 November 2026.

    fn new_york(naive: NaiveDateTime) -> DateTime<Tz> {
        New_York.from_local_datetime(&naive).earliest().expect("time should exist")
    }

    #[test]
    fn hourly_skips_the_missing_hour_in_spring() {
        let now = new_york(wall(2026, 3, 8, 1, 30, 0));
        let next = next_boundary(&interval(60.0), &now).unwrap();
        // 02:00 does not exist that night; the change lands on 03:00, 30 real minutes later.
        assert_eq!(next.naive_local(), wall(2026, 3, 8, 3, 0, 0));
        assert_eq!(next - now, Duration::minutes(30));
        let after = next_boundary(&interval(60.0), &next).unwrap();
        assert_eq!(after.naive_local(), wall(2026, 3, 8, 4, 0, 0));
    }

    #[test]
    fn daily_time_inside_the_missing_hour_moves_past_the_gap() {
        let now = new_york(wall(2026, 3, 8, 1, 0, 0));
        let next = next_boundary(&daily("02:30"), &now).unwrap();
        assert_eq!(next.naive_local(), wall(2026, 3, 8, 3, 30, 0));
        // The day after, 02:30 exists again.
        let following = next_boundary(&daily("02:30"), &next).unwrap();
        assert_eq!(following.naive_local(), wall(2026, 3, 9, 2, 30, 0));
    }

    #[test]
    fn daily_keeps_its_clock_time_across_a_daylight_saving_change() {
        let before = new_york(wall(2026, 3, 7, 12, 0, 0));
        let first = next_boundary(&daily("09:00"), &before).unwrap();
        assert_eq!(first.naive_local(), wall(2026, 3, 8, 9, 0, 0));
        let second = next_boundary(&daily("09:00"), &first).unwrap();
        assert_eq!(second.naive_local(), wall(2026, 3, 9, 9, 0, 0));
        // Only 23 real hours pass over the night the clocks jump, and still 09:00.
        let across = next_boundary(&daily("09:00"), &new_york(wall(2026, 3, 7, 9, 0, 0))).unwrap();
        assert_eq!(across - new_york(wall(2026, 3, 7, 9, 0, 0)), Duration::hours(23));
        assert_eq!(across.naive_local(), wall(2026, 3, 8, 9, 0, 0));
    }

    #[test]
    fn boundaries_stay_ahead_of_now_in_the_repeated_hour_in_autumn() {
        // 01:40 happens twice on 1 November; take the second one (standard time).
        let second_pass = New_York.from_local_datetime(&wall(2026, 11, 1, 1, 40, 0)).latest().unwrap();
        let first_pass = New_York.from_local_datetime(&wall(2026, 11, 1, 1, 40, 0)).earliest().unwrap();
        assert_eq!(second_pass - first_pass, Duration::hours(1));

        let next = next_boundary(&interval(15.0), &second_pass).unwrap();
        assert!(next > second_pass, "must be strictly after now");
        assert_eq!(next.naive_local(), wall(2026, 11, 1, 1, 45, 0));
        assert_eq!(next - second_pass, Duration::minutes(5));

        let from_first = next_boundary(&interval(15.0), &first_pass).unwrap();
        assert_eq!(from_first - first_pass, Duration::minutes(5));

        let hourly = next_boundary(&interval(60.0), &second_pass).unwrap();
        assert_eq!(hourly.naive_local(), wall(2026, 11, 1, 2, 0, 0));
        assert_eq!(hourly - second_pass, Duration::minutes(20));
    }

    #[test]
    fn every_result_is_strictly_in_the_future_across_a_whole_year() {
        // Walk a year in 7-minute strides through a zone with daylight saving.
        let mut now = new_york(wall(2026, 1, 1, 0, 0, 0));
        let end = new_york(wall(2027, 1, 1, 0, 0, 0));
        let schedules = [interval(15.0), interval(60.0), interval(180.0), interval(420.0), daily("02:30")];
        while now < end {
            for schedule in &schedules {
                let next = next_boundary(schedule, &now).unwrap();
                assert!(next > now, "{schedule:?} at {now}");
                assert!(next - now <= Duration::hours(25), "{schedule:?} at {now}");
            }
            now += Duration::minutes(7);
        }
    }
}
