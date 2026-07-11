//! Pure decision logic for the sidecar supervisor: when the health watchdog
//! trips, and whether a dead daemon may be respawned. Kept free of I/O and
//! tauri types so the policies are unit-testable.

use std::time::{Duration, Instant};

/// What a readiness probe learned.
///
/// A probe dials the daemon over a unix socket from *this* process, so it can come
/// back unhappy for two entirely different reasons, and the watchdog must never
/// confuse them: the daemon is not serving, or this process has no descriptors left
/// to dial it with.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Probe {
    /// The daemon answered.
    Healthy,
    /// The daemon was dialled and did not serve the request.
    Unserved,
    /// This process hit its own open-file limit (`EMFILE`/`ENFILE`) while dialling.
    /// Says nothing whatsoever about the daemon's health.
    LocalDescriptorExhaustion,
}

/// Whether a probe outcome is evidence *about the daemon*.
///
/// Running this process out of descriptors is evidence about this process. The
/// daemon may be serving perfectly and merely be unreachable, so such a failure
/// must never count toward the kill threshold: restarting the daemon cannot hand a
/// descriptor back, the next probe fails identically, and the only thing achieved
/// is destroying a healthy backend — and the user's session with it.
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
}

/// Extra wall-clock time beyond a nominal probe cycle still attributable to a
/// merely-busy machine rather than an OS suspension. A watchdog cycle is
/// nominally `WATCHDOG_INTERVAL + PROBE_TIMEOUT`; load can stretch it a little,
/// but App Nap or a system sleep overshoots it by far more.
const SUSPENSION_MARGIN: Duration = Duration::from_secs(15);

/// Reports whether a watchdog cycle that spanned `elapsed` wall-clock time was
/// interrupted by an OS suspension (App Nap / system sleep) rather than merely
/// running slowly. A suspended daemon is frozen, not wedged: its probe failed
/// only because it was not executing, and it answers again the instant it
/// resumes — so a failure observed across such a gap is not evidence of a wedge
/// and must not count toward the kill threshold. The caller measures `elapsed`
/// with the wall clock (SystemTime), which — unlike a monotonic Instant —
/// keeps advancing through a system sleep, so the overshoot is visible on resume.
pub fn cycle_was_suspended(elapsed: Duration, interval: Duration, probe_timeout: Duration) -> bool {
    elapsed > interval + probe_timeout + SUSPENSION_MARGIN
}

/// Final gate before the watchdog's irreversible SIGQUIT+SIGKILL restart, given
/// whether the daemon process is currently OS-suspended and whether it answered
/// a longer grace probe. macOS task-suspends an idle, backgrounded helper: while
/// suspended it fails every probe because it is not executing, yet answers the
/// instant it resumes — so a suspended daemon must never be killed (that is the
/// "backend closed itself while idle" false positive). A daemon that answers the
/// grace probe merely resumed or was transiently slow. Only a daemon that is
/// running (not suspended) AND still unresponsive is genuinely wedged.
pub fn should_kill_wedged(daemon_suspended: bool, grace_probe_healthy: bool) -> bool {
    !daemon_suspended && !grace_probe_healthy
}

/// Allows at most `max` restarts inside a sliding `window`. A daemon that
/// dies instantly on every boot (bad build, corrupt state) must not be
/// respawned in a tight loop forever; once the budget is exhausted the
/// supervisor gives up until the next app launch.
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
            should_kill_wedged(false, false),
            "running yet unresponsive after the grace probe = genuine wedge, kill"
        );
        assert!(
            !should_kill_wedged(true, false),
            "an OS-suspended daemon is frozen, not wedged — never kill it"
        );
        assert!(
            !should_kill_wedged(false, true),
            "answered the grace probe — it recovered, do not kill"
        );
        assert!(
            !should_kill_wedged(true, true),
            "suspended (and/or recovered) — do not kill"
        );
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
