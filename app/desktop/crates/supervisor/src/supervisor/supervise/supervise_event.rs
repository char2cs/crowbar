use std::fmt;

use journal::{Event, Level};

pub(crate) enum SuperviseEvent {
    Unserved,
}

impl fmt::Display for SuperviseEvent {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Unserved => {
                f.write_str("the daemon is not serving yet; it keeps running under the watchdog")
            },
        }
    }
}

impl Event for SuperviseEvent {
    fn source(&self) -> &'static str {
        "supervisor::supervisor::supervise"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Unserved => "daemon_unserved",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::Unserved => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::events::assert_events;

    #[test]
    fn an_unserved_daemon_is_a_warning() {
        assert_events([(
            SuperviseEvent::Unserved,
            "daemon_unserved",
            Level::Warn,
            "the daemon is not serving yet; it keeps running under the watchdog",
        )]);
    }
}
