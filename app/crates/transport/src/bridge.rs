use std::convert::Infallible;
use std::future::Future;
use std::time::Duration;

use futures_util::{Sink, SinkExt, Stream, StreamExt};
use tokio::io::{AsyncRead, AsyncWrite};
use tokio_tungstenite::WebSocketStream;
use tokio_tungstenite::tungstenite::protocol::CloseFrame;
use tokio_tungstenite::tungstenite::protocol::frame::coding::CloseCode;
use tokio_tungstenite::tungstenite::{Error as WsError, Message};
use tokio_util::sync::CancellationToken;

use crate::bridge_limits::BridgeLimits;
use crate::ending::Ending;
use crate::outcome::Outcome;
use crate::upstream::UpstreamSocket;

pub(crate) fn close_message(
    code: CloseCode,
    reason: &'static str,
) -> Message {
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

async fn send_within<W>(
    write: &mut W,
    message: Message,
    limit: Duration,
) -> Result<(), Ending>
where
    W: Sink<Message> + Unpin,
{
    match tokio::time::timeout(limit, write.send(message)).await {
        Ok(Ok(())) => Ok(()),
        Ok(Err(_)) => Err(Ending::Gone),
        Err(_) => Err(Ending::Stalled),
    }
}

async fn forward<W>(
    message: Message,
    write: &mut W,
    limit: Duration,
) -> Option<Ending>
where
    W: Sink<Message> + Unpin,
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
    W: Sink<Message> + Unpin,
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
    W: Sink<Message> + Unpin,
{
    let ending = match next_within(read, idle).await {
        Err(()) => Some(Ending::Idle),
        Ok(None) => Some(Ending::Gone),
        Ok(Some(Err(e))) => Some(ending_of(&e)),
        Ok(Some(Ok(message))) => forward(message, write, write_timeout).await,
    };
    ending.map_or(Ok(()), Err)
}

async fn run<R, W>(
    mut read: R,
    write: &mut W,
    idle: Option<Duration>,
    write_timeout: Duration,
) -> Result<Infallible, Ending>
where
    R: Stream<Item = Result<Message, WsError>> + Unpin,
    W: Sink<Message> + Unpin,
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
    W: Sink<Message> + Unpin,
{
    let Err(ending) = run(read, write, idle, write_timeout).await;
    ending
}

pub(crate) async fn bridge<S>(
    client: WebSocketStream<S>,
    upstream: UpstreamSocket,
    cancel: CancellationToken,
    limits: BridgeLimits,
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

async fn settle<U, D>(
    outcome: &Outcome,
    upload: &mut U,
    download: &mut D,
    grace: Duration,
) where
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

async fn say_and_close<S>(
    sink: &mut S,
    farewell: Option<Message>,
    grace: Duration,
) where
    S: Sink<Message> + Unpin,
{
    if tokio::time::timeout(grace, farewell_then_close(sink, farewell))
        .await
        .is_err()
    {
        tracing::debug!("peer did not take the close within the grace period");
    }
}

async fn farewell_then_close<S>(
    sink: &mut S,
    farewell: Option<Message>,
) where
    S: Sink<Message> + Unpin,
{
    if let Some(message) = farewell
        && sink.send(message).await.is_err()
    {
        return;
    }
    if sink.close().await.is_err() {
        tracing::debug!("peer was already gone at close");
    }
}

async fn finish<C, U>(
    outcome: Outcome,
    client: &mut C,
    upstream: &mut U,
    limits: BridgeLimits,
) where
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
mod tests {
    use std::pin::Pin;
    use std::task::{Context, Poll};

    use futures_util::stream;

    use super::*;

    const WRITE: Duration = Duration::from_secs(10);
    const GRACE: Duration = Duration::from_secs(2);

    fn limits() -> BridgeLimits {
        BridgeLimits {
            client_idle: None,
            upstream_idle: None,
            write_timeout: WRITE,
            close_grace: GRACE,
        }
    }

    fn frames(messages: Vec<Message>) -> impl Stream<Item = Result<Message, WsError>> + Unpin {
        stream::iter(messages.into_iter().map(Ok))
    }

    struct Stuck;

    impl Sink<Message> for Stuck {
        type Error = WsError;

        fn poll_ready(
            self: Pin<&mut Self>,
            _: &mut Context<'_>,
        ) -> Poll<Result<(), WsError>> {
            Poll::Pending
        }

        fn start_send(
            self: Pin<&mut Self>,
            _: Message,
        ) -> Result<(), WsError> {
            Ok(())
        }

        fn poll_flush(
            self: Pin<&mut Self>,
            _: &mut Context<'_>,
        ) -> Poll<Result<(), WsError>> {
            Poll::Pending
        }

        fn poll_close(
            self: Pin<&mut Self>,
            _: &mut Context<'_>,
        ) -> Poll<Result<(), WsError>> {
            Poll::Pending
        }
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
        let mut stuck = Stuck;
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

    #[tokio::test]
    async fn finishing_a_revoked_stream_tells_both_sides_why() {
        let (mut client, mut upstream): (Vec<Message>, Vec<Message>) = (Vec::new(), Vec::new());
        finish(Outcome::Revoked, &mut client, &mut upstream, limits()).await;
        assert_eq!(client, vec![close_message(CloseCode::Policy, "revoked")]);
        assert_eq!(
            upstream,
            vec![close_message(CloseCode::Away, "client revoked")]
        );
    }

    #[tokio::test]
    async fn an_upstream_that_vanished_is_reported_to_the_client_as_an_error() {
        let (mut client, mut upstream): (Vec<Message>, Vec<Message>) = (Vec::new(), Vec::new());
        finish(
            Outcome::Download(Ending::Gone),
            &mut client,
            &mut upstream,
            limits(),
        )
        .await;
        assert_eq!(
            client,
            vec![close_message(CloseCode::Error, "upstream closed")]
        );
        assert_eq!(upstream.len(), 0);
    }

    fn code(message: Option<Message>) -> Option<CloseCode> {
        match message {
            Some(Message::Close(Some(frame))) => Some(frame.code),
            _ => None,
        }
    }

    type Case = (Outcome, Option<CloseCode>, Option<CloseCode>);

    fn assert_farewells(cases: Vec<Case>) {
        for (outcome, to_client, to_upstream) in cases {
            let (client, upstream) = farewells(&outcome);
            assert_eq!(code(client), to_client, "client side of {outcome:?}");
            assert_eq!(code(upstream), to_upstream, "upstream side of {outcome:?}");
        }
    }

    #[test]
    fn a_client_that_ends_the_stream_is_named_by_close_code_on_both_sides() {
        assert_farewells(vec![
            (
                Outcome::Upload(Ending::TooLarge),
                Some(CloseCode::Size),
                Some(CloseCode::Away),
            ),
            (
                Outcome::Upload(Ending::Protocol),
                Some(CloseCode::Protocol),
                Some(CloseCode::Away),
            ),
            (
                Outcome::Upload(Ending::Idle),
                Some(CloseCode::Policy),
                Some(CloseCode::Away),
            ),
            (Outcome::Upload(Ending::Gone), None, Some(CloseCode::Away)),
            (
                Outcome::Upload(Ending::Stalled),
                Some(CloseCode::Error),
                None,
            ),
        ]);
    }

    #[test]
    fn an_upstream_that_ends_the_stream_is_named_by_close_code_on_both_sides() {
        assert_farewells(vec![
            (
                Outcome::Download(Ending::Protocol),
                Some(CloseCode::Error),
                None,
            ),
            (
                Outcome::Download(Ending::Idle),
                Some(CloseCode::Error),
                None,
            ),
            (
                Outcome::Download(Ending::Stalled),
                None,
                Some(CloseCode::Away),
            ),
            (Outcome::Download(Ending::Closed), None, None),
        ]);
    }

    #[tokio::test(start_paused = true)]
    async fn a_client_that_never_reads_cannot_hold_the_close_open() {
        let mut client = Stuck;
        let mut upstream: Vec<Message> = Vec::new();
        let started = tokio::time::Instant::now();
        finish(Outcome::Revoked, &mut client, &mut upstream, limits()).await;
        assert_eq!(
            started.elapsed(),
            GRACE,
            "the stall is bounded by the grace period"
        );
        assert_eq!(
            upstream,
            vec![close_message(CloseCode::Away, "client revoked")],
            "the daemon still hears the close"
        );
    }
}
