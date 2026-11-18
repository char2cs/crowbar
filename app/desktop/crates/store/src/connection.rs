//! One open stream: the text frames in, the requests out.

use tokio::sync::mpsc;

#[derive(Debug)]
pub(crate) struct Connection {
    pub(crate) incoming: mpsc::Receiver<String>,
    pub(crate) outgoing: mpsc::UnboundedSender<String>,
}
