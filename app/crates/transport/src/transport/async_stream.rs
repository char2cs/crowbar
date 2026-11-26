use tokio::io::{AsyncRead, AsyncWrite};

pub(crate) trait AsyncStream: AsyncRead + AsyncWrite + Unpin + Send {}

impl<T: AsyncRead + AsyncWrite + Unpin + Send> AsyncStream for T {}

pub(crate) type BoxedStream = Box<dyn AsyncStream>;
