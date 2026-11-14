use std::time::Duration;

/// Every timing and limit of supervision. The defaults are the production values; tests shrink
/// them.
#[derive(Debug, Clone)]
pub struct Timings {
    /// Spacing of startup health polls.
    pub health_poll: Duration,
    /// Failed startup polls before giving up on the daemon.
    pub health_attempts: u32,
    /// Budget of one startup health probe.
    pub health_probe_timeout: Duration,
    /// Spacing of the watchdog's deep readiness probes.
    pub watchdog_interval: Duration,
    /// Budget of one deep probe; it answers in microseconds when healthy.
    pub probe_timeout: Duration,
    /// Budget of the final, more generous probe before a wedge is declared.
    pub grace_probe_timeout: Duration,
    /// Consecutive failed probes before the watchdog considers a wedge.
    pub failure_threshold: u32,
    /// Slack beyond a nominal cycle still blamed on a busy machine rather than sleep.
    pub suspension_margin: Duration,
    /// Wait after SIGTERM before SIGKILL on an intentional stop.
    pub sigterm_grace: Duration,
    /// Wait after SIGQUIT (goroutine dump) before SIGKILL on a wedged daemon.
    pub sigquit_grace: Duration,
    /// Budget of the goroutine dump request.
    pub dump_timeout: Duration,
    /// First delay before a respawn; later ones double up to `respawn_cap`.
    pub respawn_base: Duration,
    /// Longest delay before a respawn.
    pub respawn_cap: Duration,
    /// Automatic respawns allowed inside `restart_window`.
    pub restart_max: usize,
    /// The sliding window `restart_max` counts in.
    pub restart_window: Duration,
    /// Size at which the daemon log rotates to `.1`.
    pub log_max_bytes: u64,
}

impl Default for Timings {
    fn default() -> Self {
        Self {
            health_poll: Duration::from_millis(200),
            health_attempts: 30,
            health_probe_timeout: Duration::from_secs(3),
            watchdog_interval: Duration::from_secs(10),
            probe_timeout: Duration::from_secs(5),
            grace_probe_timeout: Duration::from_secs(8),
            failure_threshold: 6,
            suspension_margin: Duration::from_secs(15),
            sigterm_grace: Duration::from_secs(3),
            sigquit_grace: Duration::from_secs(2),
            dump_timeout: Duration::from_secs(10),
            respawn_base: Duration::from_millis(500),
            respawn_cap: Duration::from_secs(10),
            restart_max: 3,
            restart_window: Duration::from_secs(600),
            log_max_bytes: 4 * 1024 * 1024,
        }
    }
}

#[cfg(test)]
#[path = "timings_tests.rs"]
mod tests;
