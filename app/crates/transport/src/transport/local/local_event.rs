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
    fn each_event_has_its_name_level_and_message() {
        let refused = io::Error::other("refused");
        let path = Path::new("/run/cb.sock");
        let missing = Error::NoSocket;
        let too_large = Error::TooLarge;
        let cases = [
            (
                LocalEvent::SocketNotConnectable {
                    path,
                    error: &refused,
                },
                "local_socket_not_connectable",
                "daemon socket /run/cb.sock not connectable: refused",
            ),
            (
                LocalEvent::SocketNotReady {
                    attempt: 3,
                    failure: &missing,
                },
                "local_socket_not_ready",
                "daemon socket not ready (attempt 3): the local daemon has not published a socket",
            ),
            (
                LocalEvent::DaemonUnreachable { error: &missing },
                "local_daemon_unreachable",
                "local daemon unreachable: the local daemon has not published a socket",
            ),
            (
                LocalEvent::ExchangeFailed { error: &too_large },
                "local_exchange_failed",
                "local daemon exchange failed: response too large",
            ),
        ];
        for (event, name, message) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), Level::Debug);
            assert_eq!(event.to_string(), message);
        }
    }
}
