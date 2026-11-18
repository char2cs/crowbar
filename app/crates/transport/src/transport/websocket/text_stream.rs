use std::ops::ControlFlow;

use futures_util::{SinkExt, StreamExt};
use protocol::PaneOrigin;
use tokio_tungstenite::tungstenite::{Error as WsError, Message};

use crate::error::Error;
use crate::hosts::Target;
use crate::transport::Transport;
use crate::transport::websocket::upstream::UpstreamSocket;
use crate::transport::websocket::upstream_request::UpstreamRequest;

const MEBIBYTE: usize = 1 << 20;
const MAX_FRAME: usize = 16 * MEBIBYTE;

/// A WebSocket to the local daemon that carries text frames, for the shell's own use.
pub struct TextStream {
    socket: UpstreamSocket,
}

impl std::fmt::Debug for TextStream {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("TextStream").finish_non_exhaustive()
    }
}

impl TextStream {
    pub(crate) fn new(socket: UpstreamSocket) -> Self {
        Self { socket }
    }

    /// The next text frame; `None` once the connection has ended. Other frame kinds are not
    /// frames of this stream and are skipped.
    pub async fn recv(&mut self) -> Option<String> {
        let mut flow = ControlFlow::Continue(());
        while flow.is_continue() {
            flow = classify(self.socket.next().await);
        }
        flow.break_value().flatten()
    }

    /// Sends one text frame.
    ///
    /// # Errors
    /// [`Error::WebSocket`] when the connection has ended.
    pub async fn send(
        &mut self,
        text: String,
    ) -> Result<(), Error> {
        self.socket
            .send(Message::Text(text.into()))
            .await
            .map_err(Error::websocket)
    }
}

fn classify(frame: Option<Result<Message, WsError>>) -> ControlFlow<Option<String>> {
    match frame {
        Some(Ok(Message::Text(text))) => ControlFlow::Break(Some(text.as_str().to_owned())),
        Some(Ok(Message::Close(_))) | None => ControlFlow::Break(None),
        Some(Err(error)) => {
            tracing::debug!(%error, "a daemon text stream failed");
            ControlFlow::Break(None)
        },
        Some(Ok(_)) => ControlFlow::Continue(()),
    }
}

impl Transport {
    /// Opens the daemon's WebSocket at `path` as the shell itself, not on behalf of a pane.
    ///
    /// # Errors
    /// [`Error::NoSocket`] or [`Error::LocalSocket`] while the local daemon is not reachable,
    /// [`Error::WebSocket`] when the daemon refuses the upgrade.
    pub async fn open_text_stream(
        &self,
        path: &str,
    ) -> Result<TextStream, Error> {
        let wanted = UpstreamRequest {
            target: Target::Local,
            origin: PaneOrigin::Core,
            path_and_query: path.to_owned(),
            max_message: MAX_FRAME,
        };
        Ok(TextStream::new(self.open_stream(&wanted).await?))
    }
}

#[cfg(test)]
mod tests {
    use tokio::io::duplex;
    use tokio_tungstenite::{accept_async, client_async};

    use super::*;
    use crate::transport::async_stream::BoxedStream;

    async fn pair() -> (
        TextStream,
        tokio_tungstenite::WebSocketStream<tokio::io::DuplexStream>,
    ) {
        let (near, far) = duplex(4096);
        let boxed: BoxedStream = Box::new(near);
        let (client, server) =
            tokio::join!(client_async("ws://localhost/x", boxed), accept_async(far));
        (TextStream::new(client.unwrap().0), server.unwrap())
    }

    #[tokio::test]
    async fn text_frames_cross_both_ways_and_other_kinds_are_skipped() {
        let (mut stream, mut server) = pair().await;
        stream.send("hello".into()).await.unwrap();
        let heard = server.next().await.unwrap().unwrap();
        assert_eq!(heard, Message::Text("hello".into()));
        server.send(Message::Binary(vec![1].into())).await.unwrap();
        server.send(Message::Ping(vec![2].into())).await.unwrap();
        server.send(Message::Text("back".into())).await.unwrap();
        assert_eq!(stream.recv().await.as_deref(), Some("back"));
    }

    #[tokio::test]
    async fn a_closed_connection_ends_the_stream_and_refuses_sends() {
        let (mut stream, mut server) = pair().await;
        server.close(None).await.unwrap();
        assert_eq!(stream.recv().await, None);
        drop(server);
        assert!(stream.send("late".into()).await.is_err());
    }
}
