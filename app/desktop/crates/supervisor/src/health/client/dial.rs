use std::future::Future;

use tokio::io::{AsyncRead, AsyncWrite};

pub(crate) trait Dial: Sync {
    type Stream: AsyncRead + AsyncWrite + Unpin + Send + 'static;

    fn dial(&self) -> impl Future<Output = std::io::Result<Self::Stream>> + Send;
}
