use std::future::Future;

use crate::error::Error;

/// The text frames of one connection to the daemon's browser peer socket.
pub trait PeerStream: Send + 'static {
    /// The next frame, or `None` once the connection has ended.
    fn recv(&mut self) -> impl Future<Output = Option<String>> + Send;

    /// Sends one frame.
    ///
    /// # Errors
    /// [`Error::Stream`] when the connection is gone.
    fn send(
        &mut self,
        text: String,
    ) -> impl Future<Output = Result<(), Error>> + Send;
}
