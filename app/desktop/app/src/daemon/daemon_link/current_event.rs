use supervisor::DaemonEvent;

use super::DaemonLink;

impl DaemonLink {
    pub(crate) fn current_event(&self) -> Option<DaemonEvent> {
        let supervisor = self.supervisor()?;
        let current = supervisor.current();
        let event = current.borrow().clone();
        Some(event)
    }

    pub(crate) fn heard_answer(&self) {
        if let Some(supervisor) = self.supervisor() {
            supervisor.heard_answer();
        }
    }
}

#[cfg(test)]
mod tests {
    use crate::test_support::link_without_daemon;

    #[test]
    fn a_link_that_supervises_no_daemon_knows_no_event() {
        assert_eq!(link_without_daemon().current_event(), None);
    }
}
