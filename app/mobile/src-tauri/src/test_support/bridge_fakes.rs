use std::sync::Mutex;

use tokio::io::{duplex, DuplexStream};
use tokio::sync::{mpsc, oneshot};
use tokio_tungstenite::{accept_async, client_async, WebSocketStream};

use crate::ws_bridge::frame::{Frame, WS_CLOSE_SENTINEL};
use crate::ws_bridge::frame_sink::FrameSink;

const DUPLEX_BYTES: usize = 1 << 20;

pub(crate) struct Frames(mpsc::UnboundedSender<Frame>);

impl FrameSink for Frames {
    fn send(
        &self,
        frame: Frame,
    ) {
        if self.0.send(frame).is_err() {
            log::debug!("test sink: receiver dropped");
        }
    }
}

pub(crate) fn frames() -> (Frames, mpsc::UnboundedReceiver<Frame>) {
    let (tx, rx) = mpsc::unbounded_channel();
    (Frames(tx), rx)
}

pub(crate) fn describe(frame: &Frame) -> String {
    match frame {
        Frame::Text(text) => format!("text:{text}"),
        Frame::Binary(bytes) => format!("binary:{bytes:?}"),
    }
}

pub(crate) struct ClosureSink<F>(pub(crate) F);

impl<F: Fn(Frame) + Send + Sync + 'static> FrameSink for ClosureSink<F> {
    fn send(
        &self,
        frame: Frame,
    ) {
        (self.0)(frame);
    }
}

type Announcer = Mutex<Option<oneshot::Sender<()>>>;

fn announce_close(
    slot: &Announcer,
    frame: &Frame,
) {
    if *frame != Frame::Text(WS_CLOSE_SENTINEL.to_owned()) {
        return;
    }
    let taken = slot.lock().ok().and_then(|mut guard| guard.take());
    if let Some(tx) = taken {
        tx.send(()).ok();
    }
}

pub(crate) fn sentinel_sink(tx: oneshot::Sender<()>) -> impl FrameSink {
    let slot = Mutex::new(Some(tx));
    ClosureSink(move |frame: Frame| announce_close(&slot, &frame))
}

pub(crate) fn silent_sink() -> impl FrameSink {
    ClosureSink(|_| {})
}

pub(crate) async fn ws_pair() -> (WebSocketStream<DuplexStream>, WebSocketStream<DuplexStream>) {
    let (client_io, server_io) = duplex(DUPLEX_BYTES);
    let (client, server) = tokio::join!(
        client_async("ws://localhost/v0/x", client_io),
        accept_async(server_io)
    );
    (
        client.expect("client handshake").0,
        server.expect("server handshake"),
    )
}
