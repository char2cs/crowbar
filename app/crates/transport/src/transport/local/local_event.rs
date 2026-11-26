//! What reaching the local daemon over its unix socket logs.

use std::path::Path;
use std::{fmt, io};

use journal::Level;

use crate::error::Error;

pub(crate) enum LocalEvent<'a> {
    SocketNotConnectable {
        path: &'a Path,
        error: &'a io::Error,
    },
    SocketNotReady {
        attempt: u32,
        failure: &'a Error,
    },
    DaemonUnreachable {
        error: &'a Error,
    },
    ExchangeFailed {
        error: &'a Error,
    },
}

impl fmt::Display for LocalEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::SocketNotConnectable { path, error } => {
                write!(
                    f,
                    "daemon socket {} not connectable: {error}",
                    path.display()
                )
            },
            Self::SocketNotReady { attempt, failure } => {
                write!(f, "daemon socket not ready (attempt {attempt}): {failure}")
            },
            Self::DaemonUnreachable { error } => write!(f, "local daemon unreachable: {error}"),
            Self::ExchangeFailed { error } => write!(f, "local daemon exchange failed: {error}"),
        }
    }
}

impl journal::Event for LocalEvent<'_> {
    fn source(&self) -> &'static str {
        "transport::transport::local"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::SocketNotConnectable { .. } => "local_socket_not_connectable",
            Self::SocketNotReady { .. } => "local_socket_not_ready",
            Self::DaemonUnreachable { .. } => "local_daemon_unreachable",
            Self::ExchangeFailed { .. } => "local_exchange_failed",
        }
    }

    fn level(&self) -> Level {
        Level::Debug
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn an_event_has_its_name_level_and_message() {
        let event = LocalEvent::ExchangeFailed {
            error: &Error::TooLarge,
        };
        assert_eq!(
            (event.name(), event.level(), event.to_string()),
            (
                "local_exchange_failed",
                Level::Debug,
                "local daemon exchange failed: response too large".to_owned()
            )
        );
    }
}
