mod farewell_event;

use std::fmt::Debug;
use std::future::Future;
use std::time::Duration;

use futures_util::{Sink, SinkExt};
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::tungstenite::protocol::frame::coding::CloseCode;

use farewell_event::FarewellEvent;

use crate::journal_sink::emit;
use crate::relay::session::bridge::close_message::close_message;
use crate::relay::session::bridge::ending::Ending;
use crate::relay::session::bridge::limits::Limits;
use crate::relay::session::bridge::outcome::Outcome;

pub(crate) async fn settle<U, D>(
    outcome: &Outcome,
    upload: &mut U,
    download: &mut D,
    grace: Duration,
) where
    U: Future + Unpin,
    D: Future + Unpin,
{
    match outcome {
        Outcome::Upload(Ending::Closed) => await_pump(grace, download).await,
        Outcome::Download(Ending::Closed) => await_pump(grace, upload).await,
        _ => {},
    }
}

async fn await_pump<P: Future + Unpin>(
    grace: Duration,
    pump: &mut P,
) {
    if tokio::time::timeout(grace, pump).await.is_err() {
        emit(&FarewellEvent::OtherDirectionLate);
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
    S: Sink<Message, Error: Debug> + Unpin,
{
    if tokio::time::timeout(grace, farewell_then_close(sink, farewell))
        .await
        .is_err()
    {
        emit(&FarewellEvent::CloseNotTaken);
    }
}

async fn farewell_then_close<S>(
    sink: &mut S,
    farewell: Option<Message>,
) where
    S: Sink<Message, Error: Debug> + Unpin,
{
    if let Some(message) = farewell
        && sink.send(message).await.is_err()
    {
        return;
    }
    if sink.close().await.is_err() {
        emit(&FarewellEvent::GoneAtClose);
    }
}

pub(crate) async fn finish<C, U>(
    outcome: Outcome,
    client: &mut C,
    upstream: &mut U,
    limits: Limits,
) where
    C: Sink<Message, Error: Debug> + Unpin,
    U: Sink<Message, Error: Debug> + Unpin,
{
    let (to_client, to_upstream) = farewells(&outcome);
    tokio::join!(
        say_and_close(client, to_client, limits.close_grace),
        say_and_close(upstream, to_upstream, limits.close_grace),
    );
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::relay::session::bridge::test_support::StuckSink;

    const WRITE: Duration = Duration::from_secs(10);
    const GRACE: Duration = Duration::from_secs(2);

    fn limits() -> Limits {
        Limits {
            client_idle: None,
            upstream_idle: None,
            write_timeout: WRITE,
            close_grace: GRACE,
        }
    }

    fn code(message: Option<Message>) -> Option<CloseCode> {
        match message {
            Some(Message::Close(Some(frame))) => Some(frame.code),
            _ => None,
        }
    }

    type Case = (Outcome, Option<CloseCode>, Option<CloseCode>);

    fn assert_farewells(cases: &[Case]) {
        for (outcome, to_client, to_upstream) in cases {
            let (client, upstream) = farewells(outcome);
            assert_eq!(code(client), *to_client, "client side of {outcome:?}");
            assert_eq!(code(upstream), *to_upstream, "upstream side of {outcome:?}");
        }
    }

    const CASES: [Case; 11] = [
        (
            Outcome::Revoked,
            Some(CloseCode::Policy),
            Some(CloseCode::Away),
        ),
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
            Outcome::Download(Ending::Gone),
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
    ];

    #[test]
    fn whoever_ends_the_stream_is_named_by_close_code_on_both_sides() {
        assert_farewells(&CASES);
    }

    #[tokio::test(start_paused = true)]
    async fn a_client_that_never_reads_cannot_hold_the_close_open() {
        let mut client = StuckSink::default();
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
