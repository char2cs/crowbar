use std::time::Duration;

use crate::exoneration::Exoneration;

const PERCENT_SCALE: u32 = 100;

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
    let utime: u64 = fields.get(11)?.parse().ok()?;
    let stime: u64 = fields.get(12)?.parse().ok()?;
    if ticks_per_sec == 0 {
        return None;
    }
    Some(Duration::from_millis(
        (utime + stime) * 1000 / ticks_per_sec,
    ))
}

pub(crate) fn parse_ps_cpu_time(text: &str) -> Option<Duration> {
    let text = text.trim();
    let (days, clock) = match text.split_once('-') {
        Some((days, clock)) => (days.parse::<f64>().ok()?, clock),
        None => (0.0, text),
    };
    let mut secs = days * 86_400.0;
    let mut unit = 1.0;
    for part in clock.rsplit(':') {
        secs += part.parse::<f64>().ok()? * unit;
        unit *= 60.0;
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
        assert_eq!(parse_proc_stat_cpu(stat, 100), Some(Duration::from_secs(3)));
        assert_eq!(parse_proc_stat_cpu("4242 (x) S 1", 100), None);
        assert_eq!(parse_proc_stat_cpu(stat, 0), None);
    }

    #[test]
    fn ps_cpu_time_reads_both_clock_formats() {
        assert_eq!(
            parse_ps_cpu_time("00:01:05\n"),
            Some(Duration::from_secs(65))
        );
        assert_eq!(
            parse_ps_cpu_time("2-01:00:00"),
            Some(Duration::from_hours(49))
        );
        assert_eq!(
            parse_ps_cpu_time(" 1:02.50"),
            Some(Duration::from_millis(62_500))
        );
        assert_eq!(parse_ps_cpu_time(""), None);
        assert_eq!(parse_ps_cpu_time("garbage"), None);
    }

    #[test]
    fn a_stopped_process_state_is_t() {
        assert!(proc_state_is_stopped("4242 (crowbar api) T 1 4242"));
        assert!(!proc_state_is_stopped("4242 (crowbar api) S 1 4242"));
        assert!(!proc_state_is_stopped("4242 (T) S 1"));
    }
}
