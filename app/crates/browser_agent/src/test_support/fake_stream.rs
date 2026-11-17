use std::future::Future;

use tokio::sync::mpsc;

use crate::error::Error;
use crate::peer_stream::PeerStream;

pub(crate) struct FakeStream {
    inbound: mpsc::UnboundedReceiver<String>,
    outbound: mpsc::UnboundedSender<String>,
}

impl FakeStream {
    pub(crate) fn new(
        inbound: mpsc::UnboundedReceiver<String>,
        outbound: mpsc::UnboundedSender<String>,
    ) -> Self {
        Self { inbound, outbound }
    }
}

impl PeerStream for FakeStream {
    async fn recv(&mut self) -> Option<String> {
        self.inbound.recv().await
    }

    fn send(
        &mut self,
        text: String,
    ) -> impl Future<Output = Result<(), Error>> + Send {
        let sent = self
            .outbound
            .send(text)
            .map_err(|e| Error::Stream(e.to_string()));
        std::future::ready(sent)
    }
}
