use std::sync::Arc;

use browser_agent::{Error, PeerConnector};
use transport::Transport;

use crate::peer_line::PeerLine;

const PEER_PATH: &str = "/v0/browser/peer";

pub(crate) struct DaemonPeer {
    transport: Arc<Transport>,
}

impl DaemonPeer {
    pub(crate) fn new(transport: Arc<Transport>) -> Self {
        Self { transport }
    }
}

impl PeerConnector for DaemonPeer {
    type Stream = PeerLine;

    async fn connect(&self) -> Result<PeerLine, Error> {
        let stream = self
            .transport
            .open_text_stream(PEER_PATH)
            .await
            .map_err(|error| Error::Stream(Box::new(error)))?;
        Ok(PeerLine::new(stream))
    }
}

#[cfg(test)]
mod tests {
    use browser_agent::PeerConnector;

    use super::*;
    use crate::test_support::link_without_daemon;

    #[test]
    fn without_a_daemon_the_peer_cannot_connect_and_says_so() {
        let link = link_without_daemon();
        let peer = DaemonPeer::new(Arc::clone(&link.daemon));
        let outcome = link.handle().block_on(peer.connect());
        assert!(matches!(outcome.map(|_| ()), Err(Error::Stream(_))));
    }
}
