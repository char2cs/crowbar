use std::time::Duration;

use futures_util::stream::SplitStream;
use futures_util::StreamExt;
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::sync::oneshot;
use tokio::task::JoinHandle;
use tokio::time::error::Elapsed;
use tokio::time::timeout;
use tokio_tungstenite::tungstenite::{Error as WsError, Message};
use tokio_tungstenite::WebSocketStream;

use super::frame::{Frame, WS_CLOSE_SENTINEL};
use super::frame_sink::FrameSink;
use super::ws_bridge_manager::{retire_generation, Connections};

type Read = Result<Option<Result<Message, WsError>>, Elapsed>;

pub(super) struct Session<T, S> {
    pub(super) read: SplitStream<WebSocketStream<T>>,
    pub(super) on_message: S,
    pub(super) cancelled: oneshot::Receiver<()>,
    pub(super) writer_dead: oneshot::Receiver<()>,
    pub(super) idle: Option<Duration>,
    pub(super) connections: Connections,
    pub(super) conn_id: String,
    pub(super) generation: u64,
    pub(super) writer: JoinHandle<()>,
}

async fn read_next<T>(
    read: &mut SplitStream<WebSocketStream<T>>,
    idle: Option<Duration>,
) -> Read
where
    T: AsyncRead + AsyncWrite + Unpin,
{
    match idle {
        Some(limit) => timeout(limit, read.next()).await,
        None => Ok(read.next().await),
    }
}

fn forward<S: FrameSink>(
    sink: &S,
    read: Read,
) -> Option<bool> {
    match read {
        Ok(Some(Ok(Message::Text(text)))) => {
            sink.send(Frame::Text(text.to_string()));
            None
        },
        Ok(Some(Ok(Message::Binary(bytes)))) => {
            sink.send(Frame::Binary(bytes.to_vec()));
            None
        },
        Ok(Some(Ok(_))) => None,
        Ok(Some(Err(error))) => {
            log::debug!("ws bridge: the daemon stream failed: {error}");
            Some(true)
        },
        Ok(None) => Some(true),
        Err(elapsed) => {
            log::debug!("ws bridge: the daemon stream went idle: {elapsed}");
            Some(true)
        },
    }
}

impl<T, S> Session<T, S>
where
    T: AsyncRead + AsyncWrite + Unpin + Send + 'static,
    S: FrameSink,
{
    pub(super) async fn serve(mut self) {
        let daemon_closed = self.pump().await;
        let Self {
            read,
            on_message,
            connections,
            conn_id,
            generation,
            writer,
            ..
        } = self;
        drop(read);
        if daemon_closed {
            retire_generation(&connections, &conn_id, generation);
        }
        if let Err(error) = writer.await {
            log::debug!("ws bridge: writer task ended abnormally: {error}");
        }
        if daemon_closed {
            on_message.send(Frame::Text(WS_CLOSE_SENTINEL.to_owned()));
        }
    }

    async fn pump(&mut self) -> bool {
        let mut verdict = self.step().await;
        while verdict.is_none() {
            verdict = self.step().await;
        }
        verdict.unwrap_or(true)
    }

    async fn step(&mut self) -> Option<bool> {
        let next = read_next(&mut self.read, self.idle);
        tokio::select! {
            _ = &mut self.cancelled => Some(false),
            writer_result = &mut self.writer_dead => Some(writer_result.is_ok()),
            frame = next => forward(&self.on_message, frame),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::bridge_fakes::{describe, frames};
    use std::future::pending;

    async fn read_for(kind: &str) -> Read {
        match kind {
            "text" => Ok(Some(Ok(Message::Text("hi".into())))),
            "binary" => Ok(Some(Ok(Message::Binary(vec![0u8, 240].into())))),
            "ping" => Ok(Some(Ok(Message::Ping(vec![1u8].into())))),
            "error" => Ok(Some(Err(WsError::ConnectionClosed))),
            "end" => Ok(None),
            _ => {
                timeout(
                    Duration::from_millis(1),
                    pending::<Option<Result<Message, WsError>>>(),
                )
                .await
            },
        }
    }

    #[tokio::test(start_paused = true)]
    async fn forward_relays_data_frames_and_ends_on_close_error_or_idle() {
        forward_relays_data_frames_and_ends_on_c_run().await;
    }

    async fn forward_relays_data_frames_and_ends_on_c_run() {
        type Case = (&'static str, Option<bool>, &'static str);
        const CASES: &[Case] = &[
            ("text", None, "text:hi"),
            ("binary", None, "binary:[0, 240]"),
            ("ping", None, ""),
            ("error", Some(true), ""),
            ("end", Some(true), ""),
            ("idle", Some(true), ""),
        ];
        for (kind, verdict, sent) in CASES {
            let (sink, mut received) = frames();
            let outcome = forward(&sink, read_for(kind).await);
            let got = received
                .try_recv()
                .map(|f| describe(&f))
                .unwrap_or_default();
            assert_eq!(outcome, *verdict, "{kind}");
            assert_eq!(got, *sent, "{kind}");
        }
    }
}
