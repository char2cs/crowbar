use std::fmt;

use journal::Level;

use crate::error::Error;

pub(crate) enum TerminalSessionEvent<'a> {
    Failed { error: &'a Error },
    StreamLost { reason: &'a str },
}

impl fmt::Display for TerminalSessionEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Failed { error } => write!(f, "terminal session failed error={error}"),
            Self::StreamLost { reason } => write!(f, "terminal stream lost reason={reason}"),
        }
    }
}

impl journal::Event for TerminalSessionEvent<'_> {
    fn source(&self) -> &'static str {
        "ptysession::terminal_sessions"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Failed { .. } => "terminal_session_failed",
            Self::StreamLost { .. } => "terminal_stream_lost",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::Failed { .. } | Self::StreamLost { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn each_event_has_its_name_level_and_message() {
        let error = Error::NoSessionId;
        let cases = [
            (
                TerminalSessionEvent::Failed { error: &error },
                "terminal_session_failed",
                Level::Warn,
                "terminal session failed error=terminal session: the daemon answered without a \
                 session id",
            ),
            (
                TerminalSessionEvent::StreamLost {
                    reason: "the connection dropped",
                },
                "terminal_stream_lost",
                Level::Warn,
                "terminal stream lost reason=the connection dropped",
            ),
        ];
        for (event, name, level, message) in cases {
            assert_eq!(
                (event.name(), event.level(), event.to_string().as_str()),
                (name, level, message)
            );
        }
    }
}
