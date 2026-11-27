use std::fmt;
use std::time::Duration;

use journal::{Event, Level};
use tokio::time::error::Elapsed;

use crate::error::Error;

pub(crate) enum WedgeKillEvent<'a> {
    NoDump { error: &'a Error },
    NoDumpInTime { timeout: Duration, elapsed: Elapsed },
    Unkillable { error: &'a std::io::Error },
}

impl fmt::Display for WedgeKillEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::NoDump { error } => write!(f, "no goroutine dump: {error}"),
            Self::NoDumpInTime { timeout, elapsed } => {
                write!(f, "no goroutine dump within {timeout:?}: {elapsed}")
            },
            Self::Unkillable { error } => write!(f, "cannot kill the wedged daemon: {error}"),
        }
    }
}

impl Event for WedgeKillEvent<'_> {
    fn source(&self) -> &'static str {
        "supervisor::supervisor::monitoring"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::NoDump { .. } => "goroutine_dump_failed",
            Self::NoDumpInTime { .. } => "goroutine_dump_timed_out",
            Self::Unkillable { .. } => "wedged_daemon_unkillable",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::NoDump { .. } | Self::NoDumpInTime { .. } => Level::Debug,
            Self::Unkillable { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::events::assert_events;

    #[tokio::test]
    async fn each_wedge_kill_event_has_its_name_level_and_message() {
        let elapsed = tokio::time::timeout(Duration::ZERO, std::future::pending::<()>())
            .await
            .unwrap_err();
        let refused = Error::GaveUp(1);
        let denied = std::io::Error::from(std::io::ErrorKind::PermissionDenied);
        let late = WedgeKillEvent::NoDumpInTime {
            timeout: Duration::from_secs(5),
            elapsed,
        };
        assert_events([
            (
                WedgeKillEvent::NoDump { error: &refused },
                "goroutine_dump_failed",
                Level::Debug,
                "no goroutine dump: daemon did not become healthy in 1 attempts",
            ),
            (
                late,
                "goroutine_dump_timed_out",
                Level::Debug,
                "no goroutine dump within 5s: deadline has elapsed",
            ),
            (
                WedgeKillEvent::Unkillable { error: &denied },
                "wedged_daemon_unkillable",
                Level::Warn,
                "cannot kill the wedged daemon: permission denied",
            ),
        ]);
    }
}
