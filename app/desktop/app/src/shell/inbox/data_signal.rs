use std::rc::Rc;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};

use store::{ChangeSignal, Outcome, OutcomeSink};

use crate::daemon::daemon_link::DaemonLink;
use crate::shell::inbox::shell_request::ShellRequest;

pub(crate) fn callbacks(
    requests: &async_channel::Sender<ShellRequest>,
    queued: &Arc<AtomicBool>,
) -> (ChangeSignal, OutcomeSink) {
    let (changes, flag) = (requests.clone(), Arc::clone(queued));
    let signal: ChangeSignal = Arc::new(move || queue_change(&flag, &changes));
    let outcomes = requests.clone();
    let outcome: OutcomeSink = Arc::new(move |outcome: Outcome| pass_outcome(&outcomes, outcome));
    (signal, outcome)
}

fn queue_change(
    flag: &AtomicBool,
    changes: &async_channel::Sender<ShellRequest>,
) {
    if flag.swap(true, Ordering::SeqCst) {
        return;
    }
    if changes.try_send(ShellRequest::DataChanged).is_err() {
        tracing::debug!("the shell is closing; a data change goes unseen");
    }
}

fn pass_outcome(
    outcomes: &async_channel::Sender<ShellRequest>,
    outcome: Outcome,
) {
    if outcomes.try_send(ShellRequest::Outcome(outcome)).is_err() {
        tracing::debug!("the shell is closing; a command result goes unseen");
    }
}

pub(crate) fn start_data(
    link: &DaemonLink,
    requests: &async_channel::Sender<ShellRequest>,
) -> (Rc<store::Store>, Arc<AtomicBool>) {
    let queued = Arc::new(AtomicBool::new(false));
    let (signal, outcome) = callbacks(requests, &queued);
    let wiring = store::Ports {
        desk: link.desk(),
        signal,
        outcome,
    };
    let data = store::Store::start(&link.handle(), &link.transport(), link.hosts(), wiring);
    (Rc::new(data), queued)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn changes_queue_one_redraw_until_it_is_taken_and_outcomes_always_pass() {
        let (sender, receiver) = async_channel::unbounded();
        let queued = Arc::new(AtomicBool::new(false));
        let (signal, outcome) = callbacks(&sender, &queued);
        signal();
        signal();
        assert_eq!(receiver.try_recv().unwrap(), ShellRequest::DataChanged);
        assert!(
            receiver.try_recv().is_err(),
            "the second change is coalesced"
        );
        queued.store(false, Ordering::SeqCst);
        signal();
        assert_eq!(receiver.try_recv().unwrap(), ShellRequest::DataChanged);
        outcome(Outcome::OpenChat("c".into()));
        assert_eq!(
            receiver.try_recv().unwrap(),
            ShellRequest::Outcome(Outcome::OpenChat("c".into()))
        );
    }
}
