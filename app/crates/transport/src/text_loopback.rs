use tokio::io::duplex;
use tokio_tungstenite::{accept_async, client_async};

use crate::async_stream::BoxedStream;
use crate::error::Error;
use crate::text_stream::TextStream;

const BUFFER: usize = 1 << 16;

impl TextStream {
    /// Two connected streams in memory, for tests of code that talks over a [`TextStream`].
    ///
    /// # Errors
    /// [`Error::WebSocket`] when the in-memory handshake fails.
    pub async fn loopback() -> Result<(Self, Self), Error> {
        let (near, far) = duplex(BUFFER);
        let (near, far): (BoxedStream, BoxedStream) = (Box::new(near), Box::new(far));
        let (client, server) =
            tokio::join!(client_async("ws://localhost/", near), accept_async(far));
        let (client, _) = client.map_err(Error::websocket)?;
        let server = server.map_err(Error::websocket)?;
        Ok((Self::new(client), Self::new(server)))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn what_one_end_sends_the_other_receives() {
        let (mut near, mut far) = TextStream::loopback().await.unwrap();
        near.send("ping".into()).await.unwrap();
        assert_eq!(far.recv().await.as_deref(), Some("ping"));
        far.send("pong".into()).await.unwrap();
        assert_eq!(near.recv().await.as_deref(), Some("pong"));
    }
}
