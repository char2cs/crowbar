use std::sync::Arc;

use ptysession::{Readiness, Wait};
use supervisor::DaemonEvent;
use tokio::sync::watch;

use super::DaemonLink;
use crate::link_state::LinkState;

impl DaemonLink {
    pub(crate) fn readiness(&self) -> Readiness {
        let state = Arc::clone(&self.state);
        Readiness::new(move || serving(&state))
    }
}

fn serving(state: &LinkState) -> Wait {
    let Some(supervisor) = state.supervisor() else {
        return Box::pin(async {});
    };
    Box::pin(until_ready(supervisor.current()))
}

async fn until_ready(mut current: watch::Receiver<DaemonEvent>) {
    if current.wait_for(is_ready).await.is_err() {
        tracing::debug!("the daemon's supervisor stopped before it served");
    }
}

fn is_ready(event: &DaemonEvent) -> bool {
    matches!(event, DaemonEvent::Ready { .. })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::link_state::linked;

    #[test]
    fn a_daemon_this_shell_does_not_supervise_is_taken_as_serving_at_once() {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .build()
            .unwrap();
        let (state, _locator) = linked();
        runtime.block_on(serving(&state));
    }

    #[test]
    fn only_a_serving_daemon_is_ready() {
        assert!(is_ready(&DaemonEvent::Ready { pid: None }));
        assert!(!is_ready(&DaemonEvent::Starting));
    }
}
