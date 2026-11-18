use std::time::Duration;

use crate::exoneration::Exoneration;

const PERCENT_SCALE: u32 = 100;
const UTIME_FIELD: usize = 11;
const STIME_FIELD: usize = 12;
const MILLIS_PER_SECOND: u64 = 1000;
const SECONDS_PER_DAY: f64 = 86_400.0;
const CLOCK_BASE: f64 = 60.0;

pub(crate) fn cycle_was_suspended(
    elapsed: Duration,
    interval: Duration,
    probe_timeout: Duration,
    margin: Duration,
) -> bool {
    elapsed > interval + probe_timeout + margin
}

pub(crate) fn should_kill_wedged(exonerations: &[Exoneration]) -> bool {
    exonerations.is_empty()
}

pub(crate) fn made_progress(
    before: Option<Duration>,
    after: Option<Duration>,
    stretch: Duration,
) -> bool {
    match (before, after) {
        (Some(before), Some(after)) => after.saturating_sub(before) * PERCENT_SCALE >= stretch,
        _ => false,
    }
}

pub(crate) fn parse_proc_stat_cpu(
    stat: &str,
    ticks_per_sec: u64,
) -> Option<Duration> {
    let after_name = stat.get(stat.rfind(')')? + 1..)?;
    let fields: Vec<&str> = after_name.split_whitespace().collect();
    let utime: u64 = fields.get(UTIME_FIELD)?.parse().ok()?;
    let stime: u64 = fields.get(STIME_FIELD)?.parse().ok()?;
    if ticks_per_sec == 0 {
        return None;
    }
    Some(Duration::from_millis(
        (utime + stime) * MILLIS_PER_SECOND / ticks_per_sec,
    ))
}

pub(crate) fn parse_ps_cpu_time(text: &str) -> Option<Duration> {
    let text = text.trim();
    let (days, clock) = match text.split_once('-') {
        Some((days, clock)) => (days.parse::<f64>().ok()?, clock),
        None => (0.0, text),
    };
    let mut secs = days * SECONDS_PER_DAY;
    let mut unit = 1.0;
    for part in clock.rsplit(':') {
        secs += part.parse::<f64>().ok()? * unit;
        unit *= CLOCK_BASE;
    }
    Duration::try_from_secs_f64(secs).ok()
}

pub(crate) fn proc_state_is_stopped(stat: &str) -> bool {
    stat.get(stat.rfind(')').map_or(0, |at| at + 1)..)
        .is_some_and(|rest| rest.trim_start().starts_with('T'))
}

#[cfg(test)]
mod tests {
    use super::*;

    const MINUTE: Duration = Duration::from_secs(60);

    #[test]
    fn a_cycle_far_longer_than_nominal_means_the_process_was_suspended() {
        let (interval, probe, margin) = (
            Duration::from_secs(10),
            Duration::from_secs(5),
            Duration::from_secs(15),
        );
        for secs in [10, 15, 30] {
            assert!(
                !cycle_was_suspended(Duration::from_secs(secs), interval, probe, margin),
                "{secs}"
            );
        }
        for secs in [31, 3600] {
            assert!(
                cycle_was_suspended(Duration::from_secs(secs), interval, probe, margin),
                "{secs}"
            );
        }
    }

    #[test]
    fn only_a_running_unresponsive_idle_daemon_is_killed() {
        assert!(should_kill_wedged(&[]));
        for reason in [
            Exoneration::Suspended,
            Exoneration::Answering,
            Exoneration::Busy,
        ] {
            assert!(!should_kill_wedged(&[reason]), "{reason:?}");
        }
        assert!(!should_kill_wedged(&[
            Exoneration::Suspended,
            Exoneration::Answering
        ]));
    }

    #[test]
    fn progress_is_cpu_spent_across_the_unresponsive_stretch() {
        let at = Duration::from_secs;
        assert!(made_progress(Some(at(10)), Some(at(20)), MINUTE));
        assert!(made_progress(
            Some(at(10)),
            Some(at(10) + Duration::from_millis(600)),
            MINUTE
        ));
        assert!(!made_progress(
            Some(at(10)),
            Some(at(10) + Duration::from_millis(50)),
            MINUTE
        ));
        assert!(!made_progress(None, Some(at(20)), MINUTE));
        assert!(!made_progress(Some(at(10)), None, MINUTE));
    }

    #[test]
    fn proc_stat_cpu_reads_utime_plus_stime() {
        let stat = "4242 (crowbar api) S 1 4242 4242 0 -1 4194560 100 0 0 0 250 50 0 0 20 0 9 0";
        let cases = [
            ("utime plus stime", stat, 100, Some(Duration::from_secs(3))),
            ("too few fields", "4242 (x) S 1", 100, None),
            ("no clock ticks", stat, 0, None),
        ];
        for (name, text, ticks, expected) in cases {
            assert_eq!(parse_proc_stat_cpu(text, ticks), expected, "{name}");
        }
    }

    #[test]
    fn ps_cpu_time_reads_both_clock_formats() {
        let cases = [
            ("hours", "00:01:05\n", Some(Duration::from_secs(65))),
            ("days", "2-01:00:00", Some(Duration::from_hours(49))),
            (
                "fractional",
                " 1:02.50",
                Some(Duration::from_millis(62_500)),
            ),
            ("empty", "", None),
            ("garbage", "garbage", None),
        ];
        for (name, text, expected) in cases {
            assert_eq!(parse_ps_cpu_time(text), expected, "{name}");
        }
    }

    #[test]
    fn a_stopped_process_state_is_t() {
        let cases = [
            ("stopped", "4242 (crowbar api) T 1 4242", true),
            ("sleeping", "4242 (crowbar api) S 1 4242", false),
            ("T in the name", "4242 (T) S 1", false),
        ];
        for (name, stat, expected) in cases {
            assert_eq!(proc_state_is_stopped(stat), expected, "{name}");
        }
    }
}
