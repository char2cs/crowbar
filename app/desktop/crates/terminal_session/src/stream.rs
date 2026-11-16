//! The terminal websocket: frames to the view, the view's messages to the daemon.

use std::ops::ControlFlow;
use std::pin::pin;

use async_channel::{Receiver, Sender};
use futures_util::future::{ready, select};
use futures_util::stream::{SplitSink, SplitStream};
use futures_util::{SinkExt, Stream, StreamExt};
use terminal::{ClientMessage, DaemonFrame};
use tokio::io::{AsyncRead, AsyncWrite};
use tokio_tungstenite::WebSocketStream;
use tokio_tungstenite::tungstenite::Message;

use crate::ends::Ends;
use crate::stream_end::StreamEnd;
use crate::wire::{frame_of, message_of};

pub(crate) async fn pump<S>(
    socket: WebSocketStream<S>,
    ends: Ends,
) -> StreamEnd
where
    S: AsyncRead + AsyncWrite + Unpin,
{
    let (sink, stream) = socket.split();
    let incoming = pin!(deliver_frames(stream, ends.frames));
    let outgoing = pin!(send_messages(sink, ends.messages));
    select(incoming, outgoing).await.factor_first().0
}

async fn deliver_frames<S>(
    stream: SplitStream<WebSocketStream<S>>,
    frames: Sender<DaemonFrame>,
) -> StreamEnd
where
    S: AsyncRead + AsyncWrite + Unpin,
{
    let decoded = stream
        .take_while(|message| ready(!matches!(message, Ok(Message::Close(_)))))
        .filter_map(|message| ready(message.ok().and_then(|m| frame_of(&m))));
    match relay(decoded, &frames).await {
        ControlFlow::Break(end) => end,
        ControlFlow::Continue(()) => StreamEnd::Lost,
    }
}

async fn relay(
    mut decoded: impl Stream<Item = DaemonFrame> + Unpin,
    frames: &Sender<DaemonFrame>,
) -> ControlFlow<StreamEnd> {
    while let Some(frame) = decoded.next().await {
        deliver(frame, frames).await?;
    }
    ControlFlow::Continue(())
}

async fn deliver(
    frame: DaemonFrame,
    frames: &Sender<DaemonFrame>,
) -> ControlFlow<StreamEnd> {
    let exited = matches!(frame, DaemonFrame::Exit(_));
    if frames.send(frame).await.is_err() {
        return ControlFlow::Break(StreamEnd::ViewGone);
    }
    if exited {
        return ControlFlow::Break(StreamEnd::Exited);
    }
    ControlFlow::Continue(())
}

async fn send_messages<S>(
    mut sink: SplitSink<WebSocketStream<S>, Message>,
    messages: Receiver<ClientMessage>,
) -> StreamEnd
where
    S: AsyncRead + AsyncWrite + Unpin,
{
    let mut sent = true;
    while sent && let Ok(message) = messages.recv().await {
        sent = sink.send(message_of(&message)).await.is_ok();
    }
    match sent {
        true => StreamEnd::ViewGone,
        false => StreamEnd::Lost,
    }
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
        let end = pump(socket, Ends { frames, messages }).await;
        server.await.unwrap();
        assert_eq!(end, StreamEnd::Lost);
    }

    async fn serve_exit(far: tokio::io::DuplexStream) {
        let mut daemon = accept_async(far).await.unwrap();
        let exit = Message::text(r#"{"type":"exit","code":0}"#);
        daemon.send(exit).await.unwrap();
        while daemon.next().await.is_some() {}
    }

    #[derive(Clone, Copy)]
    enum View {
        Kept,
        Dropped,
    }

    const CASES: [(View, StreamEnd); 2] = [
        (View::Kept, StreamEnd::Exited),
        (View::Dropped, StreamEnd::ViewGone),
    ];

    async fn ends_of_each_case() -> Vec<StreamEnd> {
        let mut ends = Vec::new();
        for (view, _) in CASES {
            ends.push(end_of_stream(view).await);
        }
        ends
    }

    #[tokio::test]
    async fn the_end_of_a_stream_says_whether_the_session_or_only_the_connection_ended() {
        let expected: Vec<StreamEnd> = CASES.iter().map(|(_, end)| *end).collect();
        assert_eq!(ends_of_each_case().await, expected, "exit, then view gone");
    }

    async fn end_of_stream(view: View) -> StreamEnd {
        let (near, far) = duplex(PIPE);
        tokio::spawn(serve_exit(far));
        let (socket, _) = client_async("ws://terminal/ws", near).await.unwrap();
        let (frames, view_frames) = unbounded();
        let (view_messages, messages) = unbounded();
        let keep = matches!(view, View::Kept).then_some((view_frames, view_messages));
        let end = pump(socket, Ends { frames, messages }).await;
        drop(keep);
        end
    }
}
