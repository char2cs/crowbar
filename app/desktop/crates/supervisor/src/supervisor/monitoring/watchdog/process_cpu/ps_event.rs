use std::fmt;
use std::time::Duration;

use journal::{Event, Level};
use tokio::time::error::Elapsed;

pub(crate) enum PsEvent {
    Failed {
        command: &'static str,
        error: std::io::Error,
    },
    TimedOut {
        command: &'static str,
        timeout: Duration,
        elapsed: Elapsed,
    },
}

impl fmt::Display for PsEvent {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Failed { command, error } => write!(f, "{command}: {error}"),
            Self::TimedOut {
                command,
                timeout,
                elapsed,
            } => write!(f, "{command} did not answer within {timeout:?}: {elapsed}"),
        }
    }
}

impl Event for PsEvent {
    fn source(&self) -> &'static str {
        "supervisor::supervisor::monitoring::watchdog::process_cpu"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Failed { .. } => "ps_failed",
            Self::TimedOut { .. } => "ps_timed_out",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::Failed { .. } => Level::Debug,
            Self::TimedOut { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::events::assert_events;

    #[tokio::test]
    async fn each_ps_event_has_its_name_level_and_message() {
        let elapsed = tokio::time::timeout(Duration::ZERO, std::future::pending::<()>())
            .await
            .unwrap_err();
        let failed = PsEvent::Failed {
            command: "/bin/ps",
            error: std::io::Error::from(std::io::ErrorKind::NotFound),
        };
        let timed_out = PsEvent::TimedOut {
            command: "/bin/ps",
            timeout: Duration::from_secs(2),
            elapsed,
        };
        assert_events([
            (
                failed,
                "ps_failed",
                Level::Debug,
                "/bin/ps: entity not found",
            ),
            (
                timed_out,
                "ps_timed_out",
                Level::Warn,
                "/bin/ps did not answer within 2s: deadline has elapsed",
            ),
        ]);
    }
}
