use tokio::io::{AsyncRead, AsyncWrite};

/// Anything a daemon connection can run over: unix, plain TCP and pinned TLS share one code path.
pub trait AsyncStream: AsyncRead + AsyncWrite + Unpin + Send {}

impl<T: AsyncRead + AsyncWrite + Unpin + Send> AsyncStream for T {}

/// A boxed [`AsyncStream`].
pub type BoxedStream = Box<dyn AsyncStream>;
