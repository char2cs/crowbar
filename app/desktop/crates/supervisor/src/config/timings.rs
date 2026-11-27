use std::time::Duration;

#[derive(Debug, Clone)]
pub struct Timings {
    pub health_poll: Duration,
    pub health_attempts: u32,
    pub health_probe_timeout: Duration,
    pub watchdog_interval: Duration,
    pub probe_timeout: Duration,
    pub grace_probe_timeout: Duration,
    pub failure_threshold: u32,
    pub suspension_margin: Duration,
    pub sigterm_grace: Duration,
    pub sigquit_grace: Duration,
    pub dump_timeout: Duration,
    pub respawn_base: Duration,
    pub respawn_cap: Duration,
    pub restart_max: usize,
    pub restart_window: Duration,
    pub log_max_bytes: u64,
}

const HEALTH_POLL: Duration = Duration::from_millis(200);
const HEALTH_ATTEMPTS: u32 = 30;
const HEALTH_PROBE_TIMEOUT: Duration = Duration::from_secs(3);
const WATCHDOG_INTERVAL: Duration = Duration::from_secs(10);
const PROBE_TIMEOUT: Duration = Duration::from_secs(5);
const GRACE_PROBE_TIMEOUT: Duration = Duration::from_secs(8);
const FAILURE_THRESHOLD: u32 = 6;
const SUSPENSION_MARGIN: Duration = Duration::from_secs(15);
const SIGTERM_GRACE: Duration = Duration::from_secs(3);
const SIGQUIT_GRACE: Duration = Duration::from_secs(2);
const DUMP_TIMEOUT: Duration = Duration::from_secs(10);
const RESPAWN_BASE: Duration = Duration::from_millis(500);
const RESPAWN_CAP: Duration = Duration::from_secs(10);
const RESTART_MAX: usize = 3;
const RESTART_WINDOW: Duration = Duration::from_mins(10);
const LOG_MAX_BYTES: u64 = 4 * 1024 * 1024;

impl Default for Timings {
    fn default() -> Self {
        Self {
            health_poll: HEALTH_POLL,
            health_attempts: HEALTH_ATTEMPTS,
            health_probe_timeout: HEALTH_PROBE_TIMEOUT,
            watchdog_interval: WATCHDOG_INTERVAL,
            probe_timeout: PROBE_TIMEOUT,
            grace_probe_timeout: GRACE_PROBE_TIMEOUT,
            failure_threshold: FAILURE_THRESHOLD,
            suspension_margin: SUSPENSION_MARGIN,
            sigterm_grace: SIGTERM_GRACE,
            sigquit_grace: SIGQUIT_GRACE,
            dump_timeout: DUMP_TIMEOUT,
            respawn_base: RESPAWN_BASE,
            respawn_cap: RESPAWN_CAP,
            restart_max: RESTART_MAX,
            restart_window: RESTART_WINDOW,
            log_max_bytes: LOG_MAX_BYTES,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_defaults_give_a_wedged_daemon_every_chance_before_it_is_killed() {
        let t = Timings::default();
        assert!(
            t.grace_probe_timeout > t.probe_timeout,
            "the last probe is the most generous"
        );
        assert!(t.respawn_cap >= t.respawn_base);
        assert!(t.restart_max >= 1);
        assert!(t.failure_threshold >= 2, "one slow probe is never a wedge");
        assert!(t.sigterm_grace >= Duration::from_secs(1));
    }
}
