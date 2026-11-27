use std::time::Duration;

use crate::health::MigrationProgress;

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum DaemonEvent {
    Starting,
    Migrating {
        progress: Option<MigrationProgress>,
    },
    Restarted,
    Ready {
        pid: Option<u32>,
    },
    Unhealthy,
    Recovering {
        attempt: u32,
        delay: Duration,
        reason: String,
    },
    Halted {
        reason: Option<String>,
        message: String,
    },
    GaveUp {
        report: String,
    },
    Stopped,
}
