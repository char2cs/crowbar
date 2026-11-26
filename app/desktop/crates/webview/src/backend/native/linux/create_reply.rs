use std::sync::mpsc;
use std::time::Duration;

use crate::error::{Error, Result};
use crate::view::Id;

pub(crate) struct CreateReply {
    pub(crate) reply: mpsc::Sender<Result<()>>,
    pub(crate) id: Id,
}

impl CreateReply {
    pub(crate) fn send(
        self,
        created: Result<()>,
    ) -> Option<Id> {
        let Err(mpsc::SendError(Ok(()))) = self.reply.send(created) else {
            return None;
        };
        tracing::warn!("web view created for a caller that stopped waiting");
        Some(self.id)
    }
}

pub(crate) fn wait_for<T>(
    answer: &mpsc::Receiver<Result<T>>,
    patience: Duration,
) -> Result<T> {
    match answer.recv_timeout(patience) {
        Ok(result) => result,
        Err(mpsc::RecvTimeoutError::Timeout) => Err(Error::TimedOut),
        Err(mpsc::RecvTimeoutError::Disconnected) => Err(Error::HostStopped),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const SHORT: Duration = Duration::from_millis(30);

    #[test]
    fn a_reply_that_never_comes_is_a_timeout_not_a_hang() {
        let (reply, answer) = mpsc::channel::<Result<u8>>();
        assert!(matches!(wait_for(&answer, SHORT), Err(Error::TimedOut)));
        reply.send(Ok(1)).unwrap();
        assert_eq!(
            wait_for(&answer, SHORT).unwrap(),
            1,
            "a late reply still counts"
        );
        drop(reply);
        assert!(matches!(wait_for(&answer, SHORT), Err(Error::HostStopped)));
    }

    #[test]
    fn a_creation_nobody_waits_for_gives_its_view_back_to_be_destroyed() {
        let (reply, waiting) = mpsc::channel();
        let answered = CreateReply {
            reply: reply.clone(),
            id: Id(7),
        }
        .send(Ok(()));
        assert_eq!((answered, waiting.try_recv().is_ok()), (None, true));
        drop(waiting);
        let abandoned = CreateReply { reply, id: Id(8) }.send(Ok(()));
        assert_eq!(abandoned, Some(Id(8)));
    }
}
