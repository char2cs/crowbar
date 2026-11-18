use std::future::Future;

use crate::error::Error;
use crate::link::peer_stream::PeerStream;

/// Opens connections to the daemon's browser peer socket.
pub trait PeerConnector: Send + Sync + 'static {
    /// The connections this connector opens.
    type Stream: PeerStream;

    /// Opens a connection.
    ///
    /// # Errors
    /// [`Error::Stream`] when the daemon cannot be reached.
    fn connect(&self) -> impl Future<Output = Result<Self::Stream, Error>> + Send;
}
