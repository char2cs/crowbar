//! What saying goodbye to both ends of a bridge logs.

use std::fmt;

use journal::Level;

pub(crate) enum FarewellEvent {
    OtherDirectionLate,
    CloseNotTaken,
    GoneAtClose,
}

impl fmt::Display for FarewellEvent {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        let message = match self {
            Self::OtherDirectionLate => {
                "the other direction did not finish within the grace period"
            },
            Self::CloseNotTaken => "peer did not take the close within the grace period",
            Self::GoneAtClose => "peer was already gone at close",
        };
        f.write_str(message)
    }
}

impl journal::Event for FarewellEvent {
    fn source(&self) -> &'static str {
        "transport::relay::session::bridge::farewell"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::OtherDirectionLate => "farewell_other_direction_late",
            Self::CloseNotTaken => "farewell_close_not_taken",
            Self::GoneAtClose => "farewell_gone_at_close",
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
    fn an_event_has_its_name_level_and_message() {
        let event = FarewellEvent::GoneAtClose;
        assert_eq!(
            (event.name(), event.level(), event.to_string()),
            (
                "farewell_gone_at_close",
                Level::Debug,
                "peer was already gone at close".to_owned()
            )
        );
    }
}
