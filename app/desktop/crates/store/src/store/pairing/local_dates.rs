//! The daemon's RFC 3339 times as develop shows them, in this computer's time zone.

use jiff::tz::TimeZone;
use jiff::{Timestamp, ZonedDateTime};

// SPEC: https://pkg.go.dev/time#Time.IsZero the daemon writes a time that never happened as year 1.
const FIRST_YEAR: i16 = 1;

fn parsed(
    iso: &str,
    zone: TimeZone,
) -> Option<ZonedDateTime> {
    let at: Timestamp = iso.parse().ok()?;
    let year = at.to_zoned_datetime(TimeZone::UTC).year();
    (year > FIRST_YEAR).then(|| at.to_zoned_datetime(zone))
}

pub(crate) fn day(iso: &str) -> Option<String> {
    day_in(iso, TimeZone::system())
}

pub(crate) fn time(iso: &str) -> Option<String> {
    time_in(iso, TimeZone::system())
}

fn day_in(
    iso: &str,
    zone: TimeZone,
) -> Option<String> {
    Some(parsed(iso, zone)?.strftime("%b %-d, %Y").to_string())
}

fn time_in(
    iso: &str,
    zone: TimeZone,
) -> Option<String> {
    Some(parsed(iso, zone)?.strftime("%-I:%M %p").to_string())
}

#[cfg(test)]
mod tests {
    use jiff::tz::offset;

    use super::*;

    #[test]
    fn dates_and_times_read_in_the_given_zone_and_the_zero_time_is_none() {
        let utc = || TimeZone::fixed(offset(0));
        let west = || TimeZone::fixed(offset(-3));
        let at = "2026-11-26T01:05:17Z";
        assert_eq!(day_in(at, utc()).as_deref(), Some("Nov 26, 2026"));
        assert_eq!(day_in(at, west()).as_deref(), Some("Nov 25, 2026"));
        assert_eq!(time_in(at, utc()).as_deref(), Some("1:05 AM"));
        assert_eq!(time_in(at, west()).as_deref(), Some("10:05 PM"));
        assert_eq!(day_in("0001-01-01T00:00:00Z", utc()), None);
        assert_eq!(day_in("yesterday", utc()), None);
    }
}
