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
    fn each_event_has_its_name_level_and_message() {
        let error = daemonapi::Error::Stream("reset".into());
        let problem = Problem::default();
        let cases = [
            (
                HostStreamEvent::Unopened {
                    host: "box",
                    error: &error,
                },
                "host_stream_unopened",
                format!("the stream could not be opened host=box error={error}"),
            ),
            (
                HostStreamEvent::Closed {
                    host: "box",
                    scope: "projects",
                    reason: "refused",
                    problem: Some(&problem),
                },
                "host_stream_closed",
                format!(
                    "the daemon ended a stream scope host=box scope=\"projects\" \
                     reason=\"refused\" problem=Some({problem:?})"
                ),
            ),
            (
                HostStreamEvent::Refused {
                    host: "box",
                    problem: &problem,
                },
                "host_stream_refused",
                format!("the daemon refused a stream request host=box problem={problem:?}"),
            ),
        ];
        for (event, name, message) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), Level::Warn);
            assert_eq!(event.to_string(), message);
        }
    }
}
