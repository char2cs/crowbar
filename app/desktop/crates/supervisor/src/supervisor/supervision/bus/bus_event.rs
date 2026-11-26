//! What the event bus logs as it hands each daemon event on.

use std::fmt;

use journal::{Event, Level};

use crate::supervisor::daemon_event::DaemonEvent;

pub(crate) enum BusEvent<'a> {
    Emitted { event: &'a DaemonEvent },
    Unheard,
}

impl fmt::Display for BusEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Emitted { event } => write!(f, "daemon: {event:?}"),
            Self::Unheard => f.write_str("no one is subscribed to daemon events"),
        }
    }
}

impl Event for BusEvent<'_> {
    fn source(&self) -> &'static str {
        "supervisor::supervisor::supervision::bus"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Emitted { .. } => "daemon_event",
            Self::Unheard => "daemon_event_unheard",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::Emitted { .. } => Level::Info,
            Self::Unheard => Level::Trace,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_bus_event_has_its_name_level_and_message() {
        let ready = DaemonEvent::Ready { pid: Some(7) };
        let cases = [
            (
                BusEvent::Emitted { event: &ready },
                "daemon_event",
                Level::Info,
                "daemon: Ready { pid: Some(7) }",
            ),
            (
                BusEvent::Unheard,
                "daemon_event_unheard",
                Level::Trace,
                "no one is subscribed to daemon events",
            ),
        ];
        for (event, name, level, shown) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), shown, "{name}");
        }
    }
}
