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
    use std::time::Duration;

    use journal::Event;

    use super::*;

    #[tokio::test]
    async fn each_event_has_its_name_level_and_message() {
        let late = tokio::time::timeout(Duration::ZERO, std::future::pending::<()>()).await;
        let elapsed = late.expect_err("a pending future outlasts a zero timeout");
        let refused = "sink closed";
        let cases = [
            (
                BridgeEvent::PeerRefused { refused: &refused },
                "bridge_peer_refused",
                "peer refused a message: \"sink closed\"".to_owned(),
            ),
            (
                BridgeEvent::PeerStoppedReading { elapsed: &elapsed },
                "bridge_peer_stopped_reading",
                format!("peer stopped reading: {elapsed}"),
            ),
            (
                BridgeEvent::CloseEchoNotDelivered {
                    why: Ending::Stalled,
                },
                "bridge_close_echo_not_delivered",
                format!("close echo not delivered: {:?}", Ending::Stalled),
            ),
            (
                BridgeEvent::PeerWentSilent { elapsed: &elapsed },
                "bridge_peer_went_silent",
                format!("peer went silent: {elapsed}"),
            ),
        ];
        assert_each(cases, Level::Debug);
    }

    fn assert_each<M: fmt::Display>(
        cases: impl IntoIterator<Item = (impl Event, &'static str, M)>,
        level: Level,
    ) {
        for (event, name, shown) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), shown.to_string(), "{name}");
        }
    }
}
