use std::fmt;

use journal::Level;
use tokio::time::error::Elapsed;
use tokio_tungstenite::tungstenite::Error as WsError;

use crate::error::Error;

pub(crate) enum SessionEvent<'a> {
    HandshakeRefused { error: &'a WsError },
    HandshakeTimedOut { elapsed: &'a Elapsed },
    NoAdmission,
    UpstreamRefused { error: &'a Error },
    CloseNotDelivered { error: &'a WsError },
    CloseNotTaken,
}

impl fmt::Display for SessionEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::HandshakeRefused { error } => write!(f, "relay handshake refused: {error}"),
            Self::HandshakeTimedOut { elapsed } => {
                write!(f, "relay handshake timed out: {elapsed}")
            },
            Self::NoAdmission => write!(f, "relay handshake finished without an admission"),
            Self::UpstreamRefused { error } => write!(f, "relay upstream refused: {error}"),
            Self::CloseNotDelivered { error } => write!(f, "close not delivered: {error}"),
            Self::CloseNotTaken => {
                write!(f, "client did not take the close within the grace period")
            },
        }
    }
}

impl journal::Event for SessionEvent<'_> {
    fn source(&self) -> &'static str {
        "transport::relay::session"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::HandshakeRefused { .. } => "relay_handshake_refused",
            Self::HandshakeTimedOut { .. } => "relay_handshake_timed_out",
            Self::NoAdmission => "relay_no_admission",
            Self::UpstreamRefused { .. } => "relay_upstream_refused",
            Self::CloseNotDelivered { .. } => "relay_close_not_delivered",
            Self::CloseNotTaken => "relay_close_not_taken",
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

    fn assert_each(cases: [(SessionEvent<'_>, &str, String); 6]) {
        for (event, name, message) in cases {
            assert_eq!(event.name(), name, "{message}");
            assert_eq!(event.level(), Level::Debug, "{name}");
            assert_eq!(event.to_string(), message, "{name}");
        }
    }

    #[tokio::test]
    async fn each_event_has_its_name_level_and_message() {
        let closed = WsError::ConnectionClosed;
        let late = tokio::time::timeout(Duration::ZERO, std::future::pending::<()>()).await;
        let elapsed = late.expect_err("a pending future outlasts a zero timeout");
        let refused = Error::UpgradeTimeout;
        let cases = [
            (
                SessionEvent::HandshakeRefused { error: &closed },
                "relay_handshake_refused",
                format!("relay handshake refused: {closed}"),
            ),
            (
                SessionEvent::HandshakeTimedOut { elapsed: &elapsed },
                "relay_handshake_timed_out",
                format!("relay handshake timed out: {elapsed}"),
            ),
            (
                SessionEvent::NoAdmission,
                "relay_no_admission",
                "relay handshake finished without an admission".to_owned(),
            ),
            (
                SessionEvent::UpstreamRefused { error: &refused },
                "relay_upstream_refused",
                format!("relay upstream refused: {refused}"),
            ),
            (
                SessionEvent::CloseNotDelivered { error: &closed },
                "relay_close_not_delivered",
                format!("close not delivered: {closed}"),
            ),
            (
                SessionEvent::CloseNotTaken,
                "relay_close_not_taken",
                "client did not take the close within the grace period".to_owned(),
            ),
        ];
        assert_each(cases);
    }
}
