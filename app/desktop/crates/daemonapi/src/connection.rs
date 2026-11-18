//! One open stream: the text frames in, the requests out.

use tokio::sync::mpsc;

/// One open stream: the text frames the daemon sends, and the channel for requests to it.
#[derive(Debug)]
pub struct Connection {
    /// The text frames the daemon sends, in order.
    pub incoming: mpsc::Receiver<String>,
    /// Where the shell queues its requests to the daemon.
    pub outgoing: mpsc::UnboundedSender<String>,
}
