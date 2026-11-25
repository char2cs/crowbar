//! What refusing a daemon that is not the one the supervisor started logs.

use std::fmt;

use journal::{Event, Level};

pub(crate) enum StaleDaemonEvent<'a> {
    Refused { reason: &'a str },
    Unstopped { error: &'a std::io::Error },
}

impl fmt::Display for StaleDaemonEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Refused { reason } => f.write_str(reason),
            Self::Unstopped { error } => {
                write!(f, "stopping the daemon that lost the socket: {error}")
            },
        }
    }
}

impl Event for StaleDaemonEvent<'_> {
    fn source(&self) -> &'static str {
        "supervisor::supervisor::monitoring::stale_daemon"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Refused { .. } => "stale_daemon_refused",
            Self::Unstopped { .. } => "stale_daemon_unstopped",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::Refused { .. } => Level::Error,
            Self::Unstopped { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_stale_daemon_event_has_its_name_level_and_message() {
        let denied = std::io::Error::from(std::io::ErrorKind::PermissionDenied);
        let cases = [
            (
                StaleDaemonEvent::Refused {
                    reason: "the daemon is pid 7, not 8",
                },
                "stale_daemon_refused",
                Level::Error,
                "the daemon is pid 7, not 8",
            ),
            (
                StaleDaemonEvent::Unstopped { error: &denied },
                "stale_daemon_unstopped",
                Level::Warn,
                "stopping the daemon that lost the socket: permission denied",
            ),
        ];
        for (event, name, level, shown) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), shown, "{name}");
        }
    }
}
