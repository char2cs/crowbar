//! What opening the daemon log from a startup screen tells the log.

use std::fmt;

use journal::Level;

pub(super) enum LogOpenEvent<'a> {
    NoLog,
    Unopened { error: &'a std::io::Error },
}

impl fmt::Display for LogOpenEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::NoLog => write!(f, "there is no daemon log to open"),
            Self::Unopened { error } => write!(f, "opening the daemon log failed error={error}"),
        }
    }
}

impl journal::Event for LogOpenEvent<'_> {
    fn source(&self) -> &'static str {
        "crowbar::startup_command"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::NoLog => "daemon_log_missing",
            Self::Unopened { .. } => "daemon_log_unopened",
        }
    }

    fn level(&self) -> Level {
        Level::Warn
    }
}

#[cfg(test)]
mod tests {
    use journal::Event as _;

    use super::*;

    #[test]
    fn each_log_open_event_names_itself_and_reads_as_the_old_log_line() {
        let refused = std::io::Error::other("no opener");
        let cases = [
            (
                LogOpenEvent::NoLog,
                "daemon_log_missing",
                "there is no daemon log to open",
            ),
            (
                LogOpenEvent::Unopened { error: &refused },
                "daemon_log_unopened",
                "opening the daemon log failed error=no opener",
            ),
        ];
        for (event, name, line) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), Level::Warn, "{name}");
            assert_eq!(event.to_string(), line, "{name}");
        }
    }
}
