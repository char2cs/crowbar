//! What pumping messages between a client and its upstream logs.

use std::fmt;
use std::fmt::Debug;

use journal::Level;
use tokio::time::error::Elapsed;

use crate::relay::session::bridge::ending::Ending;

pub(crate) enum BridgeEvent<'a> {
    PeerRefused { refused: &'a dyn Debug },
    PeerStoppedReading { elapsed: &'a Elapsed },
    CloseEchoNotDelivered { why: Ending },
    PeerWentSilent { elapsed: &'a Elapsed },
}

impl fmt::Display for BridgeEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::PeerRefused { refused } => write!(f, "peer refused a message: {refused:?}"),
            Self::PeerStoppedReading { elapsed } => write!(f, "peer stopped reading: {elapsed}"),
            Self::CloseEchoNotDelivered { why } => write!(f, "close echo not delivered: {why:?}"),
            Self::PeerWentSilent { elapsed } => write!(f, "peer went silent: {elapsed}"),
        }
    }
}

impl journal::Event for BridgeEvent<'_> {
    fn source(&self) -> &'static str {
        "transport::relay::session::bridge"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::PeerRefused { .. } => "bridge_peer_refused",
            Self::PeerStoppedReading { .. } => "bridge_peer_stopped_reading",
            Self::CloseEchoNotDelivered { .. } => "bridge_close_echo_not_delivered",
            Self::PeerWentSilent { .. } => "bridge_peer_went_silent",
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
        let event = BridgeEvent::CloseEchoNotDelivered {
            why: Ending::Stalled,
        };
        assert_eq!(
            (event.name(), event.level(), event.to_string()),
            (
                "bridge_close_echo_not_delivered",
                Level::Debug,
                format!("close echo not delivered: {:?}", Ending::Stalled)
            )
        );
    }
}
