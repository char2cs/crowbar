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
