use tokio::sync::{mpsc, oneshot};

use crate::error::Error;

pub(crate) enum Command {
    Restart(oneshot::Sender<Result<(), Error>>),
    Stop(oneshot::Sender<()>),
    Answered,
}

pub(crate) async fn next_command(commands: &mut mpsc::Receiver<Command>) -> Option<Command> {
    loop {
        match commands.recv().await {
            Some(Command::Answered) => {},
            other => return other,
        }
    }
}

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
    async fn heard_answers_are_passed_over_for_the_next_real_command() {
        let (send, mut commands) = mpsc::channel(4);
        let (done, _unwanted) = oneshot::channel();
        [Command::Answered, Command::Answered, Command::Stop(done)]
            .into_iter()
            .try_for_each(|command| send.try_send(command))
            .unwrap();
        drop(send);
        assert!(matches!(
            next_command(&mut commands).await,
            Some(Command::Stop(_))
        ));
        assert!(next_command(&mut commands).await.is_none());
    }

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
