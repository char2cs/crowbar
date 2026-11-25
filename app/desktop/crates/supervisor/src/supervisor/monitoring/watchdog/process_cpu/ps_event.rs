//! What asking `ps` about the daemon logs when `ps` gives no answer.

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

    #[tokio::test]
    async fn each_ps_event_has_its_name_level_and_message() {
        let elapsed = tokio::time::timeout(Duration::ZERO, std::future::pending::<()>())
            .await
            .unwrap_err();
        let cases = [
            (
                PsEvent::Failed {
                    command: "/bin/ps",
                    error: std::io::Error::from(std::io::ErrorKind::NotFound),
                },
                "ps_failed",
                Level::Debug,
                "/bin/ps: entity not found",
            ),
            (
                PsEvent::TimedOut {
                    command: "/bin/ps",
                    timeout: Duration::from_secs(2),
                    elapsed,
                },
                "ps_timed_out",
                Level::Warn,
                "/bin/ps did not answer within 2s: deadline has elapsed",
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
