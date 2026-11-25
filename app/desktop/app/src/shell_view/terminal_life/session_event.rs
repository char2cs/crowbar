//! What a terminal session that failed to start or stop tells the log.

use std::fmt;

use journal::Level;

pub(super) enum SessionEvent<'a> {
    NotStarted { reason: &'a str },
    KeptRunning { words: &'a str },
}

impl fmt::Display for SessionEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::NotStarted { reason } => write!(f, "a terminal could not start reason={reason}"),
            Self::KeptRunning { words } => {
                write!(f, "a closed terminal kept running words={words}")
            },
        }
    }
}

impl journal::Event for SessionEvent<'_> {
    fn source(&self) -> &'static str {
        "crowbar::shell_view::terminal_life"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::NotStarted { .. } => "terminal_not_started",
            Self::KeptRunning { .. } => "terminal_kept_running",
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
    fn each_session_event_names_itself_and_reads_as_the_old_log_line() {
        let cases = [
            (
                SessionEvent::NotStarted { reason: "500" },
                "terminal_not_started",
                "a terminal could not start reason=500",
            ),
            (
                SessionEvent::KeptRunning { words: "injected" },
                "terminal_kept_running",
                "a closed terminal kept running words=injected",
            ),
        ];
        for (event, name, line) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), Level::Warn, "{name}");
            assert_eq!(event.to_string(), line, "{name}");
        }
    }
}
