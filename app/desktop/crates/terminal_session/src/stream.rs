//! The terminal websocket: frames to the view, the view's messages to the daemon.

use std::pin::pin;

use async_channel::{Receiver, Sender};
use futures_util::future::{ready, select};
use futures_util::stream::{SplitSink, SplitStream};
use futures_util::{SinkExt, StreamExt};
use terminal::{ClientMessage, DaemonFrame};
use tokio::io::{AsyncRead, AsyncWrite};
use tokio_tungstenite::WebSocketStream;
use tokio_tungstenite::tungstenite::Message;

use crate::ends::Ends;
use crate::wire::{frame_of, message_of};

pub(crate) async fn pump<S>(
    socket: WebSocketStream<S>,
    ends: Ends,
) where
    S: AsyncRead + AsyncWrite + Unpin,
{
    let (sink, stream) = socket.split();
    let incoming = pin!(deliver_frames(stream, ends.frames));
    let outgoing = pin!(send_messages(sink, ends.messages));
    select(incoming, outgoing).await;
}

async fn deliver_frames<S>(
    stream: SplitStream<WebSocketStream<S>>,
    frames: Sender<DaemonFrame>,
) where
    S: AsyncRead + AsyncWrite + Unpin,
{
    let mut decoded = stream.filter_map(|message| ready(message.ok().and_then(|m| frame_of(&m))));
    while let Some(frame) = decoded.next().await
        && frames.send(frame).await.is_ok()
    {}
}

async fn send_messages<S>(
    mut sink: SplitSink<WebSocketStream<S>, Message>,
    messages: Receiver<ClientMessage>,
) where
    S: AsyncRead + AsyncWrite + Unpin,
{
    while let Ok(message) = messages.recv().await
        && sink.send(message_of(&message)).await.is_ok()
    {}
}

#[cfg(test)]
mod tests {
    use async_channel::unbounded;
    use tokio::io::duplex;
    use tokio_tungstenite::{accept_async, client_async};

    use super::*;

    const PIPE: usize = 64 * 1024;

    async fn serve_greeting(far: tokio::io::DuplexStream) -> Message {
        let mut daemon = accept_async(far).await.unwrap();
        let greeting = Message::binary(vec![0, b'h', b'i']);
        daemon.send(greeting).await.unwrap();
        daemon.next().await.unwrap().unwrap()
    }

    async fn serve_close(far: tokio::io::DuplexStream) {
        let mut daemon = accept_async(far).await.unwrap();
        daemon.close(None).await.unwrap();
    }

    #[tokio::test]
    async fn output_reaches_the_view_and_keystrokes_reach_the_daemon() {
        let (near, far) = duplex(PIPE);
        let server = tokio::spawn(serve_greeting(far));
        let (socket, _) = client_async("ws://terminal/ws", near).await.unwrap();
        let (frames, view_frames) = unbounded();
        let (view_messages, messages) = unbounded();
        tokio::spawn(pump(socket, Ends { frames, messages }));
        view_messages
            .send(ClientMessage::Input("ls".into()))
            .await
            .unwrap();
        assert_eq!(
            view_frames.recv().await.unwrap(),
            DaemonFrame::Output(b"hi".to_vec())
        );
        let received = server.await.unwrap();
        assert!(
            received.to_text().unwrap().contains("ls"),
            "keystrokes sent"
        );
    }

    #[tokio::test]
    async fn a_closed_stream_ends_the_pump() {
        let (near, far) = duplex(PIPE);
        let server = tokio::spawn(serve_close(far));
        let (socket, _) = client_async("ws://terminal/ws", near).await.unwrap();
        let (frames, _view_frames) = unbounded();
        let (_view_messages, messages) = unbounded();
        pump(socket, Ends { frames, messages }).await;
        server.await.unwrap();
    }
}
