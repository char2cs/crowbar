use std::future::Future;
use std::io;

use tokio::io::{AsyncRead, AsyncWrite};

/// Where the relay's connections come from: a loopback TCP listener in production, an in-memory
/// pipe in tests.
pub trait Acceptor: Send + 'static {
    /// One accepted connection.
    type Stream: AsyncRead + AsyncWrite + Unpin + Send + 'static;

    /// The loopback port panes dial to reach this acceptor.
    ///
    /// # Errors
    /// The port is not known.
    fn local_port(&self) -> io::Result<u16>;

    /// Waits for the next connection.
    ///
    /// # Errors
    /// The listener failed or was closed.
    fn accept(&mut self) -> impl Future<Output = io::Result<Self::Stream>> + Send;
}
