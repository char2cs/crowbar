//! What a connection's line to the daemon logs.

use std::fmt;

use journal::Level;
use tokio::task::JoinError;

use crate::error::Error;

pub(crate) enum LineEvent<'a> {
    UnknownFrame,
    ConnectionEnded { error: &'a Error },
    RequestUnanswered { error: &'a JoinError },
}

impl fmt::Display for LineEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::UnknownFrame => {
                write!(f, "the daemon sent a frame the browser peer does not know")
            },
            Self::ConnectionEnded { error } => {
                write!(f, "the daemon connection ended error={error}")
            },
            Self::RequestUnanswered { error } => {
                write!(f, "a browser request ended without an answer error={error}")
            },
        }
    }
}

impl journal::Event for LineEvent<'_> {
    fn source(&self) -> &'static str {
        "browseragent::link::line"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::UnknownFrame => "line_unknown_frame",
            Self::ConnectionEnded { .. } => "line_connection_ended",
            Self::RequestUnanswered { .. } => "line_request_unanswered",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::UnknownFrame | Self::ConnectionEnded { .. } => Level::Debug,
            Self::RequestUnanswered { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[tokio::test]
    async fn each_event_has_its_name_level_and_message() {
        let error = Error::Stream("reset".into());
        let task = tokio::spawn(std::future::pending::<()>());
        task.abort();
        let cancelled = task
            .await
            .expect_err("an aborted task ends with a join error");
        let cases = [
            (
                LineEvent::UnknownFrame,
                "line_unknown_frame",
                Level::Debug,
                "the daemon sent a frame the browser peer does not know".to_owned(),
            ),
            (
                LineEvent::ConnectionEnded { error: &error },
                "line_connection_ended",
                Level::Debug,
                "the daemon connection ended error=the daemon connection failed: reset".to_owned(),
            ),
            (
                LineEvent::RequestUnanswered { error: &cancelled },
                "line_request_unanswered",
                Level::Warn,
                format!("a browser request ended without an answer error={cancelled}"),
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
