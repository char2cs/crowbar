//! The daemon's RFC 3339 times as develop shows them, in this computer's time zone.

use chrono::{DateTime, Local, TimeZone};

// SPEC: https://pkg.go.dev/time#Time.IsZero the daemon writes a time that never happened as year 1.
const FIRST_YEAR: i32 = 1;

fn parsed<Z: TimeZone>(
    iso: &str,
    zone: &Z,
) -> Option<DateTime<Z>> {
    let at = DateTime::parse_from_rfc3339(iso).ok()?;
    let local = at.with_timezone(zone);
    (chrono::Datelike::year(&at) > FIRST_YEAR).then_some(local)
}

pub(crate) fn day(iso: &str) -> Option<String> {
    day_in(iso, &Local)
}

pub(crate) fn time(iso: &str) -> Option<String> {
    time_in(iso, &Local)
}

fn day_in<Z: TimeZone>(
    iso: &str,
    zone: &Z,
) -> Option<String>
where
    Z::Offset: std::fmt::Display,
{
    Some(parsed(iso, zone)?.format("%b %-d, %Y").to_string())
}

fn time_in<Z: TimeZone>(
    iso: &str,
    zone: &Z,
) -> Option<String>
where
    Z::Offset: std::fmt::Display,
{
    Some(parsed(iso, zone)?.format("%-I:%M %p").to_string())
}

#[cfg(test)]
mod tests {
    use chrono::FixedOffset;

    use super::*;

    #[test]
    fn dates_and_times_read_in_the_given_zone_and_the_zero_time_is_none() {
        let utc = FixedOffset::east_opt(0).unwrap();
        let west = FixedOffset::west_opt(3 * 3600).unwrap();
        let at = "2026-11-26T01:05:17Z";
        assert_eq!(day_in(at, &utc).as_deref(), Some("Nov 26, 2026"));
        assert_eq!(day_in(at, &west).as_deref(), Some("Nov 25, 2026"));
        assert_eq!(time_in(at, &utc).as_deref(), Some("1:05 AM"));
        assert_eq!(time_in(at, &west).as_deref(), Some("10:05 PM"));
        assert_eq!(day_in("0001-01-01T00:00:00Z", &utc), None);
        assert_eq!(day_in("yesterday", &utc), None);
    }
}
