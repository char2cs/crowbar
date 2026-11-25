//! What killing a wedged daemon logs when the goroutine dump or the kill does not happen.

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

    #[tokio::test]
    async fn each_wedge_kill_event_has_its_name_level_and_message() {
        let elapsed = tokio::time::timeout(Duration::ZERO, std::future::pending::<()>())
            .await
            .unwrap_err();
        let refused = Error::GaveUp(1);
        let denied = std::io::Error::from(std::io::ErrorKind::PermissionDenied);
        let cases = [
            (
                WedgeKillEvent::NoDump { error: &refused },
                "goroutine_dump_failed",
                Level::Debug,
                "no goroutine dump: daemon did not become healthy in 1 attempts",
            ),
            (
                WedgeKillEvent::NoDumpInTime {
                    timeout: Duration::from_secs(5),
                    elapsed,
                },
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
        ];
        assert_each(cases);
    }

    fn assert_each<M: fmt::Display>(
        cases: impl IntoIterator<Item = (impl Event, &'static str, Level, M)>
    ) {
        for (event, name, level, shown) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), shown.to_string(), "{name}");
        }
    }
}
