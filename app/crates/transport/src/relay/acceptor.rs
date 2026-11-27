use std::future::Future;
use std::io;

use tokio::io::{AsyncRead, AsyncWrite};

pub trait Acceptor: Send + 'static {
    type Stream: AsyncRead + AsyncWrite + Unpin + Send + 'static;

    fn local_port(&self) -> io::Result<u16>;

    fn accept(&mut self) -> impl Future<Output = io::Result<Self::Stream>> + Send;
}
