use std::future::Future;
use std::time::Duration;

use futures_util::{Sink, SinkExt, Stream, StreamExt};
use tokio::net::TcpStream;
use tokio_tungstenite::WebSocketStream;
use tokio_tungstenite::tungstenite::protocol::CloseFrame;
use tokio_tungstenite::tungstenite::protocol::frame::coding::CloseCode;
use tokio_tungstenite::tungstenite::{Error as WsError, Message};
use tokio_util::sync::CancellationToken;

use crate::ending::Ending;
use crate::outcome::Outcome;
use crate::relay_config::RelayConfig;
use crate::upstream::UpstreamSocket;

const CLOSE_GRACE: Duration = Duration::from_secs(2);

pub(crate) fn close_message(code: CloseCode, reason: &'static str) -> Message {
    Message::Close(Some(CloseFrame {
        code,
        reason: reason.into(),
    }))
}

fn echoed_close(frame: Option<CloseFrame>) -> Message {
    match frame {
        Some(frame) if frame.code.is_allowed() => Message::Close(Some(frame)),
        _ => Message::Close(None),
    }
}

async fn next_within<R>(
    read: &mut R,
    idle: Option<Duration>,
) -> Result<Option<Result<Message, WsError>>, ()>
where
    R: Stream<Item = Result<Message, WsError>> + Unpin,
{
    match idle {
        Some(limit) => tokio::time::timeout(limit, read.next())
            .await
            .map_err(|_| ()),
        None => Ok(read.next().await),
    }
}

async fn forward<W>(message: Message, write: &mut W) -> Option<Ending>
where
    W: Sink<Message> + Unpin,
{
    match message {
        Message::Text(_) | Message::Binary(_) => {
            write.send(message).await.err().map(|_| Ending::Gone)
        },
        Message::Close(frame) => {
            write.send(echoed_close(frame)).await.ok();
            Some(Ending::Closed)
        },
        _ => None,
    }
}

async fn pump<R, W>(mut read: R, write: &mut W, idle: Option<Duration>) -> Ending
where
    R: Stream<Item = Result<Message, WsError>> + Unpin,
    W: Sink<Message> + Unpin,
{
    loop {
        if let Some(ending) = step(&mut read, write, idle).await {
            return ending;
        }
    }
}

async fn step<R, W>(read: &mut R, write: &mut W, idle: Option<Duration>) -> Option<Ending>
where
    R: Stream<Item = Result<Message, WsError>> + Unpin,
    W: Sink<Message> + Unpin,
{
    match next_within(read, idle).await {
        Err(()) => Some(Ending::Idle),
        Ok(None | Some(Err(_))) => Some(Ending::Gone),
        Ok(Some(Ok(message))) => forward(message, write).await,
    }
}

pub(crate) async fn bridge(
    client: WebSocketStream<TcpStream>,
    upstream: UpstreamSocket,
    cancel: CancellationToken,
    config: &RelayConfig,
) {
    let (mut client_write, client_read) = client.split();
    let (mut upstream_write, upstream_read) = upstream.split();
    let mut upload = Box::pin(pump(client_read, &mut upstream_write, config.client_idle));
    let mut download = Box::pin(pump(upstream_read, &mut client_write, config.upstream_idle));
    let outcome = tokio::select! {
        ending = &mut upload => Outcome::Upload(ending),
        ending = &mut download => Outcome::Download(ending),
        () = cancel.cancelled() => Outcome::Revoked,
    };
    settle(&outcome, &mut upload, &mut download).await;
    drop((upload, download));
    finish(outcome, &mut client_write, &mut upstream_write).await;
}

async fn settle<U, D>(outcome: &Outcome, upload: &mut U, download: &mut D)
where
    U: Future + Unpin,
    D: Future + Unpin,
{
    match outcome {
        Outcome::Upload(Ending::Closed) => {
            tokio::time::timeout(CLOSE_GRACE, download).await.ok();
        },
        Outcome::Download(Ending::Closed) => {
            tokio::time::timeout(CLOSE_GRACE, upload).await.ok();
        },
        _ => {},
    }
}

async fn finish<C, U>(outcome: Outcome, client: &mut C, upstream: &mut U)
where
    C: Sink<Message> + Unpin,
    U: Sink<Message> + Unpin,
{
    match outcome {
        Outcome::Revoked => {
            client
                .send(close_message(CloseCode::Policy, "revoked"))
                .await
                .ok();
            upstream
                .send(close_message(CloseCode::Away, "client revoked"))
                .await
                .ok();
        },
        Outcome::Download(Ending::Gone) => {
            client
                .send(close_message(CloseCode::Error, "upstream closed"))
                .await
                .ok();
        },
        Outcome::Download(Ending::Idle) => {
            client
                .send(close_message(CloseCode::Error, "upstream idle"))
                .await
                .ok();
        },
        Outcome::Upload(Ending::Gone | Ending::Idle) => {
            upstream
                .send(close_message(CloseCode::Away, "client gone"))
                .await
                .ok();
        },
        Outcome::Upload(Ending::Closed) | Outcome::Download(Ending::Closed) => {},
    }
    client.close().await.ok();
    upstream.close().await.ok();
}

#[cfg(test)]
#[path = "bridge_tests.rs"]
mod tests;
