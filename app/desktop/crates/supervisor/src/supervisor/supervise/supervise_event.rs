//! What supervising a launched daemon logs about how its startup ended.

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

    type Case = (SuperviseEvent, &'static str, Level, &'static str);

    const CASES: [Case; 1] = [(
        SuperviseEvent::Unserved,
        "daemon_unserved",
        Level::Warn,
        "the daemon is not serving yet; it keeps running under the watchdog",
    )];

    #[test]
    fn each_supervise_event_has_its_name_level_and_message() {
        for (event, name, level, shown) in CASES {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), shown, "{name}");
        }
    }
}
