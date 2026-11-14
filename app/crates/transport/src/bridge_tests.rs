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

    fn poll_ready(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<Result<(), WsError>> {
        Poll::Pending
    }

    fn start_send(self: Pin<&mut Self>, _: Message) -> Result<(), WsError> {
        Ok(())
    }

    fn poll_flush(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<Result<(), WsError>> {
        Poll::Pending
    }

    fn poll_close(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<Result<(), WsError>> {
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

#[tokio::test]
async fn an_oversized_or_malformed_message_is_named_not_reported_as_a_dropped_socket() {
    use tokio_tungstenite::tungstenite::error::{CapacityError, ProtocolError};
    for (error, ending) in [
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
    ] {
        let mut out: Vec<Message> = Vec::new();
        let input = stream::iter(vec![Err::<Message, WsError>(error)]);
        assert_eq!(pump(input, &mut out, None, WRITE).await, ending);
    }
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
fn a_pane_that_ends_the_stream_is_named_by_close_code_on_both_sides() {
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
