use browser::AgentAnswer;
use tokio::sync::oneshot;

use crate::delivery::Delivery;
use crate::reply_scope::ReplyScope;

#[derive(Debug)]
pub(crate) struct PendingReply {
    pub(crate) serial: u64,
    pub(crate) scope: ReplyScope,
    pub(crate) sender: oneshot::Sender<AgentAnswer>,
}

impl PendingReply {
    pub(crate) fn deliver(
        self,
        answer: AgentAnswer,
    ) -> Delivery {
        self.sender
            .send(answer)
            .map_or(Delivery::NotWaiting, |()| Delivery::Delivered)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn pending() -> (PendingReply, oneshot::Receiver<AgentAnswer>) {
        let (sender, receiver) = oneshot::channel();
        let scope = ReplyScope::Shell;
        let serial = 0;
        (
            PendingReply {
                serial,
                scope,
                sender,
            },
            receiver,
        )
    }

    #[test]
    fn an_answer_is_delivered_while_someone_listens_and_not_after() {
        let (listened, receiver) = pending();
        let answer = AgentAnswer::Done("ok".into());
        assert_eq!(listened.deliver(answer.clone()), Delivery::Delivered);
        assert_eq!(receiver.blocking_recv().ok(), Some(answer.clone()));
        let (abandoned, receiver) = pending();
        drop(receiver);
        assert_eq!(abandoned.deliver(answer), Delivery::NotWaiting);
    }
}
