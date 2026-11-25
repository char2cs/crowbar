pub(crate) mod close_message;
mod ending;
mod farewell;
pub(crate) mod limits;
mod outcome;
#[cfg(test)]
mod test_support;

use std::convert::Infallible;
use std::fmt::Debug;
use std::time::Duration;

use futures_util::{Sink, SinkExt, Stream, StreamExt};
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::time::error::Elapsed;
use tokio_tungstenite::WebSocketStream;
use tokio_tungstenite::tungstenite::protocol::CloseFrame;
use tokio_tungstenite::tungstenite::{Error as WsError, Message};
use tokio_util::sync::CancellationToken;

use crate::relay::session::bridge::ending::Ending;
use crate::relay::session::bridge::farewell::{finish, settle};
use crate::relay::session::bridge::limits::Limits;
use crate::relay::session::bridge::outcome::Outcome;
use crate::transport::UpstreamSocket;

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
) -> Result<Option<Result<Message, WsError>>, Elapsed>
where
    R: Stream<Item = Result<Message, WsError>> + Unpin,
{
    match idle {
        Some(limit) => tokio::time::timeout(limit, read.next()).await,
        None => Ok(read.next().await),
    }
}

async fn send_within<W>(
    write: &mut W,
    message: Message,
    limit: Duration,
) -> Result<(), Ending>
where
    W: Sink<Message, Error: Debug> + Unpin,
{
    match tokio::time::timeout(limit, write.send(message)).await {
        Ok(Ok(())) => Ok(()),
        Ok(Err(refused)) => Err(peer_refused(&refused)),
        Err(elapsed) => Err(peer_stopped_reading(&elapsed)),
    }
}

fn peer_refused<E: Debug>(refused: &E) -> Ending {
    tracing::debug!("peer refused a message: {refused:?}");
    Ending::Gone
}

fn peer_stopped_reading(elapsed: &Elapsed) -> Ending {
    tracing::debug!("peer stopped reading: {elapsed}");
    Ending::Stalled
}

async fn forward<W>(
    message: Message,
    write: &mut W,
    limit: Duration,
) -> Option<Ending>
where
    W: Sink<Message, Error: Debug> + Unpin,
{
    match message {
        Message::Text(_) | Message::Binary(_) => send_within(write, message, limit).await.err(),
        Message::Close(frame) => {
            echo_close(write, frame, limit).await;
            Some(Ending::Closed)
        },
        _ => None,
    }
}

async fn echo_close<W>(
    write: &mut W,
    frame: Option<CloseFrame>,
    limit: Duration,
) where
    W: Sink<Message, Error: Debug> + Unpin,
{
    if let Err(why) = send_within(write, echoed_close(frame), limit).await {
        tracing::debug!("close echo not delivered: {why:?}");
    }
}

async fn step<R, W>(
    read: &mut R,
    write: &mut W,
    idle: Option<Duration>,
    write_timeout: Duration,
) -> Result<(), Ending>
where
    R: Stream<Item = Result<Message, WsError>> + Unpin,
    W: Sink<Message, Error: Debug> + Unpin,
{
    let ending = match next_within(read, idle).await {
        Err(elapsed) => Some(peer_went_silent(&elapsed)),
        Ok(None) => Some(Ending::Gone),
        Ok(Some(Err(e))) => Some(ending_of(&e)),
        Ok(Some(Ok(message))) => forward(message, write, write_timeout).await,
    };
    ending.map_or(Ok(()), Err)
}

fn peer_went_silent(elapsed: &Elapsed) -> Ending {
    tracing::debug!("peer went silent: {elapsed}");
    Ending::Idle
}

async fn run<R, W>(
    mut read: R,
    write: &mut W,
    idle: Option<Duration>,
    write_timeout: Duration,
) -> Result<Infallible, Ending>
where
    R: Stream<Item = Result<Message, WsError>> + Unpin,
    W: Sink<Message, Error: Debug> + Unpin,
{
    loop {
        step(&mut read, write, idle, write_timeout).await?;
    }
}

async fn pump<R, W>(
    read: R,
    write: &mut W,
    idle: Option<Duration>,
    write_timeout: Duration,
) -> Ending
where
    R: Stream<Item = Result<Message, WsError>> + Unpin,
    W: Sink<Message, Error: Debug> + Unpin,
{
    let Err(ending) = run(read, write, idle, write_timeout).await;
    ending
}

pub(crate) async fn bridge<S>(
    client: WebSocketStream<S>,
    upstream: UpstreamSocket,
    cancel: CancellationToken,
    limits: Limits,
) where
    S: AsyncRead + AsyncWrite + Unpin,
{
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

#[cfg(test)]
mod tests {
    use futures_util::stream;

    use tokio_tungstenite::tungstenite::protocol::frame::coding::CloseCode;

    use super::*;
    use crate::relay::session::bridge::close_message::close_message;
    use crate::relay::session::bridge::test_support::StuckSink;

    const WRITE: Duration = Duration::from_secs(10);

    fn frames(messages: Vec<Message>) -> impl Stream<Item = Result<Message, WsError>> + Unpin {
        stream::iter(messages.into_iter().map(Ok))
    }

    #[tokio::test]
    async fn text_and_binary_are_forwarded_as_the_same_kind() {
        let mut out: Vec<Message> = Vec::new();
        let input = frames(vec![
            Message::Text("hi".into()),
            Message::Binary(vec![0u8, 255, 7].into()),
            Message::Ping(vec![1].into()),
        ]);
        let ending = pump(input, &mut out, None, WRITE).await;
        assert_eq!(ending, Ending::Gone);
        assert_eq!(
            out,
            vec![
                Message::Text("hi".into()),
                Message::Binary(vec![0u8, 255, 7].into())
            ]
        );
    }

    #[tokio::test]
    async fn a_close_is_echoed_with_its_code_and_ends_the_pump_as_closed() {
        let mut out: Vec<Message> = Vec::new();
        let close = close_message(CloseCode::Normal, "bye");
        let ending = pump(
            frames(vec![close.clone(), Message::Text("late".into())]),
            &mut out,
            None,
            WRITE,
        )
        .await;
        assert_eq!(ending, Ending::Closed);
        assert_eq!(out, vec![close]);
    }

    #[tokio::test]
    async fn a_close_code_that_may_not_cross_the_wire_is_replaced_by_a_bare_close() {
        let mut out: Vec<Message> = Vec::new();
        let reserved = Message::Close(Some(CloseFrame {
            code: CloseCode::Abnormal,
            reason: "x".into(),
        }));
        pump(frames(vec![reserved]), &mut out, None, WRITE).await;
        assert_eq!(out, vec![Message::Close(None)]);
    }

    #[tokio::test(start_paused = true)]
    async fn a_silent_peer_ends_the_pump_as_idle() {
        let mut out: Vec<Message> = Vec::new();
        let silent = stream::pending::<Result<Message, WsError>>();
        let ending = pump(
            Box::pin(silent),
            &mut out,
            Some(Duration::from_secs(30)),
            WRITE,
        )
        .await;
        assert_eq!(ending, Ending::Idle);
    }

    #[tokio::test(start_paused = true)]
    async fn a_peer_that_stops_reading_ends_the_pump_as_stalled_at_the_write_deadline() {
        let mut stuck = StuckSink::default();
        let input = frames(vec![Message::Text("never taken".into())]);
        let started = tokio::time::Instant::now();
        let ending = pump(input, &mut stuck, None, WRITE).await;
        assert_eq!(ending, Ending::Stalled);
        assert_eq!(started.elapsed(), WRITE);
    }

    async fn assert_named_endings() {
        use tokio_tungstenite::tungstenite::error::{CapacityError, ProtocolError};
        let cases = [
            (
                WsError::Capacity(CapacityError::MessageTooLong {
                    size: 2,
                    max_size: 1,
                }),
                Ending::TooLarge,
            ),
            (
                WsError::Protocol(ProtocolError::ResetWithoutClosingHandshake),
                Ending::Protocol,
            ),
            (WsError::ConnectionClosed, Ending::Gone),
        ];
        for (error, ending) in cases {
            let mut out: Vec<Message> = Vec::new();
            let input = stream::iter(vec![Err::<Message, WsError>(error)]);
            assert_eq!(pump(input, &mut out, None, WRITE).await, ending);
        }
    }

    #[tokio::test]
    async fn an_oversized_or_malformed_message_is_named_not_reported_as_a_dropped_socket() {
        assert_named_endings().await;
    }
}
