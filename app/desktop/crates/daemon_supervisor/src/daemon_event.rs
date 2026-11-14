use std::time::Duration;

/// What the supervisor tells the shell, in the order a user would experience it. The shell shows
/// "Backend unavailable" for everything but `Ready`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DaemonEvent {
    /// The daemon is being spawned or has not answered its health check yet.
    Starting,
    /// The daemon serves.
    Ready {
        /// The daemon's own pid.
        pid: Option<u32>,
    },
    /// The watchdog found the daemon wedged and is killing it.
    Unhealthy,
    /// The daemon died and will be respawned after `delay`.
    Recovering {
        /// Which automatic respawn this is, from 1.
        attempt: u32,
        /// Time until the respawn.
        delay: Duration,
        /// How the last run ended.
        reason: String,
    },
    /// The daemon halted on purpose (an incompatible home); no restart fixes it.
    Halted {
        /// What the daemon said.
        message: String,
    },
    /// The daemon keeps dying and is no longer respawned until the user restarts it.
    GaveUp {
        /// Versions, a summary and the last run's log, ready to show and copy.
        report: String,
    },
    /// The daemon was stopped on purpose.
    Stopped,
}
