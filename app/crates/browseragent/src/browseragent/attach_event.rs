//! What attaching the window to the daemon's browser peer socket logs.

use std::fmt;

use journal::Level;

use crate::error::Error;

pub(crate) enum AttachEvent<'a> {
    DaemonUnreachable { error: &'a Error },
}

impl fmt::Display for AttachEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::DaemonUnreachable { error } => {
                write!(
                    f,
                    "the browser peer could not reach the daemon error={error}"
                )
            },
        }
    }
}

impl journal::Event for AttachEvent<'_> {
    fn source(&self) -> &'static str {
        "browseragent::browseragent"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::DaemonUnreachable { .. } => "peer_daemon_unreachable",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::DaemonUnreachable { .. } => Level::Debug,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn each_event_has_its_name_level_and_message() {
        let error = Error::Stream("refused".into());
        let cases = [(
            AttachEvent::DaemonUnreachable { error: &error },
            "peer_daemon_unreachable",
            Level::Debug,
            "the browser peer could not reach the daemon \
             error=the daemon connection failed: refused",
        )];
        for (event, name, level, message) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level);
            assert_eq!(event.to_string(), message);
        }
    }
}
