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
    use crate::test_support::events::assert_events;

    #[test]
    fn an_emitted_event_is_logged_at_info_with_its_value() {
        let ready = DaemonEvent::Ready { pid: Some(7) };
        assert_events([(
            BusEvent::Emitted { event: &ready },
            "daemon_event",
            Level::Info,
            "daemon: Ready { pid: Some(7) }",
        )]);
    }
}
