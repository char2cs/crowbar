use tokio::sync::oneshot;

pub(crate) fn reply<T>(
    sender: oneshot::Sender<T>,
    value: T,
) {
    if sender.send(value).is_err() {
        tracing::debug!("a supervisor command was abandoned before its answer");
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn a_reply_reaches_the_asker_and_one_that_went_away_is_not_a_failure() {
        let (sender, receiver) = oneshot::channel();
        reply(sender, 7);
        assert_eq!(receiver.await.unwrap(), 7);
        let (sender, receiver) = oneshot::channel::<u8>();
        drop(receiver);
        reply(sender, 7);
    }
}
