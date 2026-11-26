//! What following a host's stream logs when the dial fails or the daemon ends or refuses a request.

use std::fmt;

use daemonapi::Problem;
use journal::Level;

pub(crate) enum HostStreamEvent<'a> {
    Unopened {
        host: &'a str,
        error: &'a daemonapi::Error,
    },
    Closed {
        host: &'a str,
        scope: &'a str,
        reason: &'a str,
        problem: Option<&'a Problem>,
    },
    Refused {
        host: &'a str,
        problem: &'a Problem,
    },
}

impl fmt::Display for HostStreamEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Unopened { host, error } => {
                write!(
                    f,
                    "the stream could not be opened host={host} error={error}"
                )
            },
            Self::Closed {
                host,
                scope,
                reason,
                problem,
            } => write!(
                f,
                "the daemon ended a stream scope host={host} scope={scope:?} reason={reason:?} \
                 problem={problem:?}"
            ),
            Self::Refused { host, problem } => write!(
                f,
                "the daemon refused a stream request host={host} problem={problem:?}"
            ),
        }
    }
}

impl journal::Event for HostStreamEvent<'_> {
    fn source(&self) -> &'static str {
        "store::live::fleet::host_stream"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Unopened { .. } => "host_stream_unopened",
            Self::Closed { .. } => "host_stream_closed",
            Self::Refused { .. } => "host_stream_refused",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::Unopened { .. } | Self::Closed { .. } | Self::Refused { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn a_stream_that_cannot_open_warns_naming_the_host_and_the_cause() {
        let error = daemonapi::Error::Stream("reset".into());
        let event = HostStreamEvent::Unopened {
            host: "box",
            error: &error,
        };
        let message = format!("the stream could not be opened host=box error={error}");
        let seen = (event.name(), event.level(), event.to_string());
        assert_eq!(seen, ("host_stream_unopened", Level::Warn, message));
    }
}
