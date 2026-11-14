use std::time::Duration;

use crate::migration_progress::MigrationProgress;

/// What the supervisor tells the shell, in the order a user would experience it. Every variant
/// but `Ready` means the layout must not be shown: `Starting` and `Migrating` get the full
/// screen status view, `Halted` its halt card, the rest "Backend unavailable".
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum DaemonEvent {
    /// The daemon is being spawned or has not answered its health check yet.
    Starting,
    /// The daemon answers `/v0/health` with `migrating`, as often as its step changes.
    Migrating {
        /// The step it is on; `None` before the first step.
        progress: Option<MigrationProgress>,
    },
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
        /// The machine-readable halt reason (`layout_newer`, `step_failed`, ...), when given.
        reason: Option<String>,
        /// What the daemon said.
        message: String,
    },
    /// The daemon keeps dying and is no longer respawned until the user restarts it.
    GaveUp {
        /// Version, platform, a summary and the last run's log, ready to show and copy.
        report: String,
    },
    /// The daemon was stopped on purpose.
    Stopped,
}
