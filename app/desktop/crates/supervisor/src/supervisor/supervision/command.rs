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
}
