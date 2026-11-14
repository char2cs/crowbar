use futures_util::stream;

use super::*;

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
    let ending = pump(input, &mut out, None).await;
    assert!(matches!(ending, Ending::Gone));
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
    )
    .await;
    assert!(matches!(ending, Ending::Closed));
    assert_eq!(out, vec![close]);
}

#[tokio::test]
async fn a_close_code_that_may_not_cross_the_wire_is_replaced_by_a_bare_close() {
    let mut out: Vec<Message> = Vec::new();
    let reserved = Message::Close(Some(CloseFrame {
        code: CloseCode::Abnormal,
        reason: "x".into(),
    }));
    pump(frames(vec![reserved]), &mut out, None).await;
    assert_eq!(out, vec![Message::Close(None)]);
}

#[tokio::test(start_paused = true)]
async fn a_silent_peer_ends_the_pump_as_idle() {
    let mut out: Vec<Message> = Vec::new();
    let silent = stream::pending::<Result<Message, WsError>>();
    let ending = pump(Box::pin(silent), &mut out, Some(Duration::from_secs(30))).await;
    assert!(matches!(ending, Ending::Idle));
}

#[tokio::test]
async fn finishing_a_revoked_stream_tells_both_sides_why() {
    let (mut client, mut upstream): (Vec<Message>, Vec<Message>) = (Vec::new(), Vec::new());
    finish(Outcome::Revoked, &mut client, &mut upstream).await;
    assert_eq!(client, vec![close_message(CloseCode::Policy, "revoked")]);
    assert_eq!(
        upstream,
        vec![close_message(CloseCode::Away, "client revoked")]
    );
}

#[tokio::test]
async fn an_upstream_that_vanished_is_reported_to_the_client_as_an_error() {
    let (mut client, mut upstream): (Vec<Message>, Vec<Message>) = (Vec::new(), Vec::new());
    finish(Outcome::Download(Ending::Gone), &mut client, &mut upstream).await;
    assert_eq!(
        client,
        vec![close_message(CloseCode::Error, "upstream closed")]
    );
    assert_eq!(upstream.len(), 0);
}
