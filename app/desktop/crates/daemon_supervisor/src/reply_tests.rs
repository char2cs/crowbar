use super::*;

#[tokio::test]
async fn a_reply_reaches_the_asker() {
    let (sender, receiver) = oneshot::channel();
    reply(sender, 7);
    assert_eq!(receiver.await.unwrap(), 7);
}

#[test]
fn an_asker_that_went_away_is_not_a_failure() {
    let (sender, receiver) = oneshot::channel::<u8>();
    drop(receiver);
    reply(sender, 7);
}
