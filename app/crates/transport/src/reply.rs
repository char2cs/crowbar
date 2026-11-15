use protocol::{DaemonResponse, Responder};

pub(crate) struct Reply(Option<Responder>);

impl Reply {
    pub(crate) fn new(respond: Responder) -> Self {
        Self(Some(respond))
    }

    pub(crate) fn send(
        mut self,
        response: DaemonResponse,
    ) {
        if let Some(respond) = self.0.take() {
            respond(Ok(response));
        }
    }
}

impl Drop for Reply {
    fn drop(&mut self) {
        if let Some(respond) = self.0.take() {
            respond(Err(protocol::Error::Interrupted));
        }
    }
}

#[cfg(test)]
mod tests {
    use std::sync::mpsc;

    use super::*;

    fn reply() -> (Reply, mpsc::Receiver<protocol::Result<DaemonResponse>>) {
        let (sender, receiver) = mpsc::channel();
        let respond: Responder = Box::new(move |result| {
            sender.send(result).unwrap();
        });
        (Reply::new(respond), receiver)
    }

    #[test]
    fn an_answered_reply_sends_exactly_once() {
        let (reply, receiver) = reply();
        reply.send(DaemonResponse::new(204, Vec::new(), Vec::new()));
        assert_eq!(receiver.recv().unwrap().unwrap().status(), 204);
        assert!(receiver.try_recv().is_err());
    }

    #[test]
    fn a_reply_dropped_unanswered_says_it_was_interrupted() {
        let (reply, receiver) = reply();
        drop(reply);
        assert!(matches!(
            receiver.recv().unwrap(),
            Err(protocol::Error::Interrupted)
        ));
    }
}
