//! Pure decision logic for the sidecar supervisor: when the health watchdog
//! trips, and whether a dead daemon may be respawned. Kept free of I/O and
//! tauri types so the policies are unit-testable.

use std::time::{Duration, Instant};

/// What a readiness probe learned. A failure has two causes the watchdog must never
/// confuse: the daemon is not serving, or this process has no descriptors left to dial.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Probe {
    /// The daemon answered.
    Healthy,
    /// The daemon was dialled and did not serve the request.
    Unserved,
    /// This process hit its own open-file limit (`EMFILE`/`ENFILE`) while dialling.
    /// Says nothing whatsoever about the daemon's health.
    LocalDescriptorExhaustion,
    /// The daemon answered that it is alive but deliberately not serving: it is
    /// preparing its home, or it halted. No restart fixes either.
    Held,
}

/// Whether a probe outcome is evidence about the daemon. Descriptor exhaustion is ours:
/// restarting the daemon cannot hand one back, so it must never count toward a kill.
pub fn probe_indicts_daemon(outcome: Probe) -> bool {
    matches!(outcome, Probe::Unserved)
}

/// Trips after N consecutive failed probes. A single healthy probe resets the
/// streak, so transient blips (one slow request during a heavy git operation)
/// never trigger the kill path — only a sustained wedge does.
pub struct FailureTracker {
    threshold: u32,
    consecutive: u32,
}

impl FailureTracker {
    pub fn new(threshold: u32) -> Self {
        Self {
            threshold,
            consecutive: 0,
        }
    }

    /// Records one probe result. Returns true exactly once, on the probe that
    /// reaches the threshold — repeated failures beyond it stay false so the
    /// caller acts once per wedge, not once per probe.
    pub fn observe(&mut self, healthy: bool) -> bool {
        if healthy {
            self.consecutive = 0;
            return false;
        }
        self.consecutive = self.consecutive.saturating_add(1);
        self.consecutive == self.threshold
    }

    /// Clears the streak after the caller has acted on a trip.
    pub fn reset(&mut self) {
        self.consecutive = 0;
    }

    /// Feeds one readiness outcome. Only an unserved daemon counts toward a wedge:
    /// a held one is alive on purpose and a local descriptor failure says nothing
    /// about the daemon, so both clear the streak.
    pub fn observe_probe(&mut self, outcome: Probe) -> bool {
        if outcome == Probe::Unserved {
            return self.observe(false);
        }
        self.reset();
        false
    }
}

/// Slack beyond a nominal cycle (`WATCHDOG_INTERVAL + PROBE_TIMEOUT`) still blamed on a
/// busy machine; App Nap or sleep overshoots it by far more.
const SUSPENSION_MARGIN: Duration = Duration::from_secs(15);

/// Whether a cycle spanning wall-clock `elapsed` was interrupted by App Nap or sleep. A
/// suspended daemon is frozen, not wedged, so its failure must not count; `elapsed` is
/// SystemTime because, unlike Instant, it advances through a system sleep.
pub fn cycle_was_suspended(elapsed: Duration, interval: Duration, probe_timeout: Duration) -> bool {
    elapsed > interval + probe_timeout + SUSPENSION_MARGIN
}

/// Final gate before the irreversible SIGQUIT+SIGKILL. A suspended daemon fails every
/// probe yet answers on resume, one that answers the grace probe merely resumed, and one
/// that kept burning CPU is busy: only a running, unresponsive, idle daemon is wedged.
pub fn should_kill_wedged(
    daemon_suspended: bool,
    grace_probe_healthy: bool,
    made_progress: bool,
) -> bool {
    !daemon_suspended && !grace_probe_healthy && !made_progress
}

/// Whether the daemon worked through an unresponsive `stretch`: it used at least 1% of a
/// core. A deadlocked daemon idles; a loaded one computes. An unreadable sample is no
/// evidence of work, so a genuine wedge is still caught where CPU time cannot be read.
pub fn made_progress(before: Option<Duration>, after: Option<Duration>, stretch: Duration) -> bool {
    match (before, after) {
        (Some(before), Some(after)) => after.saturating_sub(before) * 100 >= stretch,
        _ => false,
    }
}

/// CPU time (user + system) from a Linux `/proc/<pid>/stat` line. Fields are counted after
/// the last `)`, since the command name in parentheses may itself hold spaces.
#[cfg(any(target_os = "linux", test))]
pub fn parse_proc_stat_cpu(stat: &str, ticks_per_sec: u64) -> Option<Duration> {
    let fields: Vec<&str> = stat
        .get(stat.rfind(')')? + 1..)?
        .split_whitespace()
        .collect();
    // utime and stime are fields 14 and 15 of the line; the state (field 3) comes first here.
    let utime: u64 = fields.get(11)?.parse().ok()?;
    let stime: u64 = fields.get(12)?.parse().ok()?;
    if ticks_per_sec == 0 {
        return None;
    }
    Some(Duration::from_millis(
        (utime + stime) * 1000 / ticks_per_sec,
    ))
}

/// CPU time from `ps -o time=`: `[DD-]HH:MM:SS` on Linux, `M:SS.cc` on macOS.
#[cfg(any(not(target_os = "linux"), test))]
pub fn parse_ps_cpu_time(text: &str) -> Option<Duration> {
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

/// The daemon log's last run, from its final spawn marker, cut to the last `max_lines`:
/// what the daemon said before it died.
pub fn last_run_tail(log: &str, max_lines: usize) -> String {
    let run = log.rfind("=== daemon spawned").map_or(log, |at| &log[at..]);
    let lines: Vec<&str> = run.lines().collect();
    lines[lines.len().saturating_sub(max_lines)..].join("\n")
}

/// At most `max` restarts per sliding `window`, so a daemon that dies on every boot is
/// not respawned forever; once spent, the supervisor gives up until the next launch.
pub struct RestartBudget {
    window: Duration,
    max: usize,
    restarts: Vec<Instant>,
}

impl RestartBudget {
    pub fn new(max: usize, window: Duration) -> Self {
        Self {
            window,
            max,
            restarts: Vec::new(),
        }
    }

    /// Returns whether a restart may happen at `now`, recording it if allowed.
    pub fn allow(&mut self, now: Instant) -> bool {
        let window = self.window;
        self.restarts
            .retain(|t| now.saturating_duration_since(*t) < window);
        if self.restarts.len() >= self.max {
            return false;
        }
        self.restarts.push(now);
        true
    }

    /// Forgets every recorded restart: one the user asked for starts a fresh budget.
    pub fn reset(&mut self) {
        self.restarts.clear();
    }
}

/// Rotates `path` to `path.1` (replacing any previous `.1`) once it reaches
/// `max_len` bytes, so the daemon log never grows unbounded but the previous
/// generation — which may hold a crash's final words — survives one rotation.
pub fn rotate_if_needed(path: &std::path::Path, max_len: u64) -> std::io::Result<()> {
    let len = match std::fs::metadata(path) {
        Ok(meta) => meta.len(),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(()),
        Err(e) => return Err(e),
    };
    if len < max_len {
        return Ok(());
    }
    let mut rotated = path.as_os_str().to_owned();
    rotated.push(".1");
    std::fs::rename(path, std::path::PathBuf::from(rotated))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_an_unserved_probe_indicts_the_daemon() {
        assert!(
            probe_indicts_daemon(Probe::Unserved),
            "the daemon was reached and did not serve — that is its failure"
        );
        assert!(!probe_indicts_daemon(Probe::Healthy));
        assert!(
            !probe_indicts_daemon(Probe::LocalDescriptorExhaustion),
            "the app ran out of descriptors; the daemon may be perfectly healthy \
             and killing it cannot hand one back"
        );
    }

    #[test]
    fn a_held_daemon_is_never_indicted() {
        assert!(
            !probe_indicts_daemon(Probe::Held),
            "a daemon preparing its home, or halted, is alive on purpose"
        );
    }

    #[test]
    fn only_an_unserved_probe_counts_toward_a_wedge() {
        let mut t = FailureTracker::new(2);
        assert!(!t.observe_probe(Probe::Unserved));
        assert!(
            !t.observe_probe(Probe::Held),
            "a held probe clears the streak"
        );
        assert!(!t.observe_probe(Probe::Unserved));
        assert!(!t.observe_probe(Probe::LocalDescriptorExhaustion));
        assert!(!t.observe_probe(Probe::Unserved));
        assert!(t.observe_probe(Probe::Unserved), "two in a row trip it");
        assert!(!t.observe_probe(Probe::Healthy));
    }

    #[test]
    fn failure_tracker_trips_exactly_once_at_threshold() {
        let mut t = FailureTracker::new(3);
        assert!(!t.observe(false));
        assert!(!t.observe(false));
        assert!(t.observe(false), "third consecutive failure must trip");
        assert!(!t.observe(false), "further failures must not re-trip");
    }

    #[test]
    fn failure_tracker_resets_on_healthy_probe() {
        let mut t = FailureTracker::new(3);
        assert!(!t.observe(false));
        assert!(!t.observe(false));
        assert!(!t.observe(true), "healthy probe resets the streak");
        assert!(!t.observe(false));
        assert!(!t.observe(false));
        assert!(t.observe(false), "streak counts from the reset");
    }

    #[test]
    fn failure_tracker_reset_allows_a_new_trip() {
        let mut t = FailureTracker::new(2);
        assert!(!t.observe(false));
        assert!(t.observe(false));
        t.reset();
        assert!(!t.observe(false));
        assert!(t.observe(false), "after reset the tracker can trip again");
    }

    #[test]
    fn cycle_was_suspended_ignores_normal_and_heavy_load_cycles() {
        let interval = Duration::from_secs(10);
        let probe = Duration::from_secs(5);
        assert!(!cycle_was_suspended(
            Duration::from_secs(10),
            interval,
            probe
        ));
        assert!(!cycle_was_suspended(
            Duration::from_secs(15),
            interval,
            probe
        ));
        assert!(
            !cycle_was_suspended(Duration::from_secs(30), interval, probe),
            "a cycle at interval+probe+margin is heavy load, not suspension"
        );
    }

    #[test]
    fn cycle_was_suspended_flags_app_nap_and_system_sleep_gaps() {
        let interval = Duration::from_secs(10);
        let probe = Duration::from_secs(5);
        assert!(
            cycle_was_suspended(Duration::from_secs(31), interval, probe),
            "a 31s cycle for a 10s sleep means the process was not executing"
        );
        assert!(
            cycle_was_suspended(Duration::from_secs(3600), interval, probe),
            "an hour-long gap is a system sleep"
        );
    }

    #[test]
    fn should_kill_only_a_running_and_still_unresponsive_daemon() {
        assert!(
            should_kill_wedged(false, false, false),
            "running, idle and unresponsive after the grace probe = genuine wedge, kill"
        );
        assert!(
            !should_kill_wedged(true, false, false),
            "an OS-suspended daemon is frozen, not wedged — never kill it"
        );
        assert!(
            !should_kill_wedged(false, true, false),
            "answered the grace probe — it recovered, do not kill"
        );
        assert!(
            !should_kill_wedged(true, true, false),
            "suspended (and/or recovered) — do not kill"
        );
    }

    #[test]
    fn a_slow_daemon_that_keeps_working_is_never_killed() {
        assert!(
            !should_kill_wedged(false, false, true),
            "it failed every probe under load but burned CPU throughout: busy, not wedged"
        );
    }

    #[test]
    fn progress_is_cpu_spent_across_the_unresponsive_stretch() {
        let minute = Duration::from_secs(60);
        let at = Duration::from_secs;
        assert!(made_progress(Some(at(10)), Some(at(20)), minute));
        assert!(
            made_progress(
                Some(at(10)),
                Some(at(10) + Duration::from_millis(600)),
                minute
            ),
            "1% of a core over a minute is work"
        );
        assert!(
            !made_progress(
                Some(at(10)),
                Some(at(10) + Duration::from_millis(50)),
                minute
            ),
            "a deadlocked Go runtime still ticks a little; that is not progress"
        );
        assert!(
            !made_progress(None, Some(at(20)), minute),
            "an unreadable sample is no evidence of work"
        );
        assert!(!made_progress(Some(at(10)), None, minute));
    }

    #[test]
    fn proc_stat_cpu_reads_utime_plus_stime() {
        let stat = "4242 (crowbar api) S 1 4242 4242 0 -1 4194560 100 0 0 0 250 50 0 0 20 0 9 0";
        assert_eq!(
            parse_proc_stat_cpu(stat, 100),
            Some(Duration::from_secs(3)),
            "300 ticks at 100 Hz, despite the space in the command name"
        );
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
            Some(Duration::from_secs(2 * 86_400 + 3_600))
        );
        assert_eq!(
            parse_ps_cpu_time(" 1:02.50"),
            Some(Duration::from_millis(62_500)),
            "macOS reports minutes and centiseconds"
        );
        assert_eq!(parse_ps_cpu_time(""), None);
        assert_eq!(parse_ps_cpu_time("garbage"), None);
    }

    #[test]
    fn the_crash_report_is_the_last_run_of_the_log() {
        let log =
            "=== daemon spawned (epoch 1)\n[err] old run\n=== daemon terminated (code=Some(2))\n\
                   === daemon spawned (epoch 2)\n[err] panic: boom\n[err] goroutine 1\n\
                   === daemon terminated (code=Some(2))\n";
        assert_eq!(
            last_run_tail(log, 10),
            "=== daemon spawned (epoch 2)\n[err] panic: boom\n[err] goroutine 1\n\
             === daemon terminated (code=Some(2))"
        );
        assert_eq!(
            last_run_tail(log, 2),
            "[err] goroutine 1\n=== daemon terminated (code=Some(2))"
        );
        assert_eq!(last_run_tail("no marker\nat all", 5), "no marker\nat all");
    }

    #[test]
    fn a_manual_restart_gets_a_fresh_budget() {
        let mut b = RestartBudget::new(2, Duration::from_secs(600));
        let now = Instant::now();
        assert!(b.allow(now));
        assert!(b.allow(now));
        assert!(!b.allow(now), "the crash loop spent the budget");
        b.reset();
        assert!(b.allow(now), "the user asked for this restart");
    }

    #[test]
    fn restart_budget_allows_up_to_max_within_window() {
        let mut b = RestartBudget::new(3, Duration::from_secs(600));
        let now = Instant::now();
        assert!(b.allow(now));
        assert!(b.allow(now));
        assert!(b.allow(now));
        assert!(!b.allow(now), "fourth restart inside the window is denied");
    }

    #[test]
    fn restart_budget_frees_slots_as_the_window_slides() {
        let mut b = RestartBudget::new(2, Duration::from_secs(600));
        let start = Instant::now();
        assert!(b.allow(start));
        assert!(b.allow(start));
        assert!(!b.allow(start + Duration::from_secs(1)));
        assert!(
            b.allow(start + Duration::from_secs(601)),
            "restarts older than the window must not count"
        );
    }

    #[test]
    fn rotate_if_needed_moves_full_log_aside() {
        // Opens files, so it must not run while a descriptor count is being taken.
        let _serialised = crate::test_support::fd_tests_blocking();

        let dir = std::env::temp_dir().join(format!("crowbar-rotate-test-{}", std::process::id()));
        std::fs::create_dir_all(&dir).unwrap();
        let log = dir.join("daemon.log");

        // Missing file: nothing to do, no error.
        rotate_if_needed(&log, 10).unwrap();

        // Below the cap: left in place.
        std::fs::write(&log, b"short").unwrap();
        rotate_if_needed(&log, 10).unwrap();
        assert!(log.exists());
        assert!(!dir.join("daemon.log.1").exists());

        // At/over the cap: rotated to .1, replacing any previous generation.
        std::fs::write(&log, b"0123456789ABCDEF").unwrap();
        rotate_if_needed(&log, 10).unwrap();
        assert!(!log.exists());
        assert_eq!(
            std::fs::read(dir.join("daemon.log.1")).unwrap(),
            b"0123456789ABCDEF"
        );

        std::fs::remove_dir_all(&dir).unwrap();
    }
}
