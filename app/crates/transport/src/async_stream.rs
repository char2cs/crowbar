use tokio::io::{AsyncRead, AsyncWrite};

pub(crate) trait AsyncStream: AsyncRead + AsyncWrite + Unpin + Send {}

impl<T: AsyncRead + AsyncWrite + Unpin + Send> AsyncStream for T {}

pub(crate) type BoxedStream = Box<dyn AsyncStream>;

#[cfg(test)]
mod tests {
    use tokio::io::{AsyncReadExt, AsyncWriteExt, duplex};

    use super::*;

    #[tokio::test]
    async fn a_boxed_stream_carries_bytes_both_ways() {
        let (near, mut far) = duplex(16);
        let mut boxed: BoxedStream = Box::new(near);
        boxed.write_all(b"ping").await.unwrap();
        let mut heard = [0u8; 4];
        far.read_exact(&mut heard).await.unwrap();
        assert_eq!(&heard, b"ping");
    }
}
