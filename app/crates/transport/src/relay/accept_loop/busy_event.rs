//! What turning a connection away from a busy relay logs.

use std::{fmt, io};

use journal::Level;
use tokio::time::error::Elapsed;

pub(crate) enum BusyEvent<'a> {
    NotDelivered { error: &'a io::Error },
    NotTaken { elapsed: &'a Elapsed },
}

impl fmt::Display for BusyEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::NotDelivered { error } => {
                write!(
                    f,
                    "relay is busy and the refusal was not delivered: {error}"
                )
            },
            Self::NotTaken { elapsed } => {
                write!(
                    f,
                    "relay is busy and the refusal was not taken in time: {elapsed}"
                )
            },
        }
    }
}

impl journal::Event for BusyEvent<'_> {
    fn source(&self) -> &'static str {
        "transport::relay::accept_loop"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::NotDelivered { .. } => "relay_busy_refusal_not_delivered",
            Self::NotTaken { .. } => "relay_busy_refusal_not_taken",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::NotDelivered { .. } | Self::NotTaken { .. } => Level::Debug,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn an_event_has_its_name_level_and_message() {
        let event = BusyEvent::NotDelivered {
            error: &io::Error::other("reset"),
        };
        assert_eq!(
            (event.name(), event.level(), event.to_string()),
            (
                "relay_busy_refusal_not_delivered",
                Level::Debug,
                "relay is busy and the refusal was not delivered: reset".to_owned()
            )
        );
    }
}
