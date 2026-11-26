//! What a session logs while it waits to reattach its lost stream.

use std::fmt;
use std::time::Duration;

use journal::Level;

pub(crate) enum RecoveryEvent<'a> {
    Reattaching { reason: &'a str, delay: Duration },
}

impl fmt::Display for RecoveryEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Reattaching { reason, delay } => {
                write!(
                    f,
                    "reattaching the terminal reason={reason} delay={delay:?}"
                )
            },
        }
    }
}

impl journal::Event for RecoveryEvent<'_> {
    fn source(&self) -> &'static str {
        "ptysession::stream::recovery"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Reattaching { .. } => "recovery_reattaching",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::Reattaching { .. } => Level::Debug,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn each_event_has_its_name_level_and_message() {
        let cases = [(
            RecoveryEvent::Reattaching {
                reason: "the connection dropped",
                delay: Duration::from_millis(250),
            },
            "recovery_reattaching",
            Level::Debug,
            "reattaching the terminal reason=the connection dropped delay=250ms",
        )];
        for (event, name, level, message) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level);
            assert_eq!(event.to_string(), message);
        }
    }
}
