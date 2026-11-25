//! What answering one request logs when the client is gone.

use std::fmt;

use journal::{Event, Level};

pub(crate) enum AnswerEvent<'a> {
    ClientLeft { error: &'a std::io::Error },
}

impl fmt::Display for AnswerEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::ClientLeft { error } => write!(f, "the client left before its answer: {error}"),
        }
    }
}

impl Event for AnswerEvent<'_> {
    fn source(&self) -> &'static str {
        "fake_daemon"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::ClientLeft { .. } => "client_left",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::ClientLeft { .. } => Level::Debug,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_answer_event_has_its_name_level_and_message() {
        let gone = std::io::Error::from(std::io::ErrorKind::BrokenPipe);
        let cases = [(
            AnswerEvent::ClientLeft { error: &gone },
            "client_left",
            Level::Debug,
            "the client left before its answer: broken pipe",
        )];
        for (event, name, level, shown) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), shown, "{name}");
        }
    }
}
