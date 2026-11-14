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

pub(crate) fn parse_proc_stat_cpu(stat: &str, ticks_per_sec: u64) -> Option<Duration> {
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
#[path = "wedge_tests.rs"]
mod tests;
