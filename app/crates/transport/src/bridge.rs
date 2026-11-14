use std::future::Future;
use std::time::Duration;

use futures_util::{Sink, SinkExt, Stream, StreamExt};
use tokio::net::TcpStream;
use tokio_tungstenite::WebSocketStream;
use tokio_tungstenite::tungstenite::protocol::CloseFrame;
use tokio_tungstenite::tungstenite::protocol::frame::coding::CloseCode;
use tokio_tungstenite::tungstenite::{Error as WsError, Message};
use tokio_util::sync::CancellationToken;

use crate::bridge_limits::BridgeLimits;
use crate::ending::Ending;
use crate::outcome::Outcome;
use crate::upstream::UpstreamSocket;

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

fn ending_of(error: &WsError) -> Ending {
    match error {
        WsError::Capacity(_) => Ending::TooLarge,
        WsError::Protocol(_) => Ending::Protocol,
        _ => Ending::Gone,
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

async fn send_within<W>(write: &mut W, message: Message, limit: Duration) -> Result<(), Ending>
where
    W: Sink<Message> + Unpin,
{
    match tokio::time::timeout(limit, write.send(message)).await {
        Ok(Ok(())) => Ok(()),
        Ok(Err(_)) => Err(Ending::Gone),
        Err(_) => Err(Ending::Stalled),
    }
}

async fn forward<W>(message: Message, write: &mut W, limit: Duration) -> Option<Ending>
where
    W: Sink<Message> + Unpin,
{
    match message {
        Message::Text(_) | Message::Binary(_) => send_within(write, message, limit).await.err(),
        Message::Close(frame) => {
            if let Err(why) = send_within(write, echoed_close(frame), limit).await {
                tracing::debug!("close echo not delivered: {why:?}");
            }
            Some(Ending::Closed)
        },
        _ => None,
    }
}

async fn pump<R, W>(
    mut read: R,
    write: &mut W,
    idle: Option<Duration>,
    write_timeout: Duration,
) -> Ending
where
    R: Stream<Item = Result<Message, WsError>> + Unpin,
    W: Sink<Message> + Unpin,
{
    loop {
        let ending = match next_within(&mut read, idle).await {
            Err(()) => Some(Ending::Idle),
            Ok(None) => Some(Ending::Gone),
            Ok(Some(Err(e))) => Some(ending_of(&e)),
            Ok(Some(Ok(message))) => forward(message, write, write_timeout).await,
        };
        if let Some(ending) = ending {
            return ending;
        }
    }
}

pub(crate) async fn bridge(
    client: WebSocketStream<TcpStream>,
    upstream: UpstreamSocket,
    cancel: CancellationToken,
    limits: BridgeLimits,
) {
    let (mut client_write, client_read) = client.split();
    let (mut upstream_write, upstream_read) = upstream.split();
    let mut upload = Box::pin(pump(
        client_read,
        &mut upstream_write,
        limits.client_idle,
        limits.write_timeout,
    ));
    let mut download = Box::pin(pump(
        upstream_read,
        &mut client_write,
        limits.upstream_idle,
        limits.write_timeout,
    ));
    let outcome = tokio::select! {
        ending = &mut upload => Outcome::Upload(ending),
        ending = &mut download => Outcome::Download(ending),
        () = cancel.cancelled() => Outcome::Revoked,
    };
    settle(&outcome, &mut upload, &mut download, limits.close_grace).await;
    drop((upload, download));
    finish(outcome, &mut client_write, &mut upstream_write, limits).await;
}

async fn settle<U, D>(outcome: &Outcome, upload: &mut U, download: &mut D, grace: Duration)
where
    U: Future + Unpin,
    D: Future + Unpin,
{
    match outcome {
        Outcome::Upload(Ending::Closed) => {
            tokio::time::timeout(grace, download).await.ok();
        },
        Outcome::Download(Ending::Closed) => {
            tokio::time::timeout(grace, upload).await.ok();
        },
        _ => {},
    }
}

type Farewells = (Option<Message>, Option<Message>);

fn farewells(outcome: &Outcome) -> Farewells {
    match outcome {
        Outcome::Revoked => (
            Some(close_message(CloseCode::Policy, "revoked")),
            Some(close_message(CloseCode::Away, "client revoked")),
        ),
        Outcome::Download(Ending::Stalled) => {
            (None, Some(close_message(CloseCode::Away, "client stalled")))
        },
        Outcome::Download(ending) => (download_farewell(*ending), None),
        Outcome::Upload(ending) => upload_farewells(*ending),
    }
}

fn download_farewell(ending: Ending) -> Option<Message> {
    let reason = match ending {
        Ending::Gone => "upstream closed",
        Ending::Idle => "upstream idle",
        Ending::TooLarge | Ending::Protocol => "upstream sent a message that cannot be forwarded",
        Ending::Closed | Ending::Stalled => return None,
    };
    Some(close_message(CloseCode::Error, reason))
}

fn upload_farewells(ending: Ending) -> Farewells {
    let away = |reason| Some(close_message(CloseCode::Away, reason));
    match ending {
        Ending::Gone => (None, away("client gone")),
        Ending::Idle => (
            Some(close_message(CloseCode::Policy, "idle")),
            away("client idle"),
        ),
        Ending::TooLarge => (
            Some(close_message(CloseCode::Size, "message too big")),
            away("client sent too much"),
        ),
        Ending::Protocol => (
            Some(close_message(CloseCode::Protocol, "protocol error")),
            away("client broke the protocol"),
        ),
        Ending::Stalled => (
            Some(close_message(CloseCode::Error, "upstream stalled")),
            None,
        ),
        Ending::Closed => (None, None),
    }
}

async fn say_and_close<S>(sink: &mut S, farewell: Option<Message>, grace: Duration)
where
    S: Sink<Message> + Unpin,
{
    let closing = async {
        if let Some(message) = farewell
            && sink.send(message).await.is_err()
        {
            return;
        }
        if sink.close().await.is_err() {
            tracing::debug!("peer was already gone at close");
        }
    };
    if tokio::time::timeout(grace, closing).await.is_err() {
        tracing::debug!("peer did not take the close within the grace period");
    }
}

async fn finish<C, U>(outcome: Outcome, client: &mut C, upstream: &mut U, limits: BridgeLimits)
where
    C: Sink<Message> + Unpin,
    U: Sink<Message> + Unpin,
{
    let (to_client, to_upstream) = farewells(&outcome);
    tokio::join!(
        say_and_close(client, to_client, limits.close_grace),
        say_and_close(upstream, to_upstream, limits.close_grace),
    );
}

#[cfg(test)]
#[path = "bridge_tests.rs"]
mod tests;
