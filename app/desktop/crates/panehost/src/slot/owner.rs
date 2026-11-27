use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use protocol::PaneId;

use crate::slot::owned_pane::OwnedPane;

#[derive(Debug, Clone, Default)]
pub(crate) struct Owner {
    state: Arc<Mutex<OwnedPane>>,
}

impl Owner {
    pub(crate) fn assign(
        &self,
        pane: PaneId,
    ) -> u64 {
        let mut state = self.lock();
        state.generation += 1;
        state.pane = Some(pane);
        state.generation
    }

    pub(crate) fn clear(&self) {
        let mut state = self.lock();
        state.generation += 1;
        state.pane = None;
    }

    pub(crate) fn current(&self) -> Option<PaneId> {
        self.lock().pane.clone()
    }

    pub(crate) fn resolve(
        &self,
        generation: u64,
    ) -> Option<PaneId> {
        let state = self.lock();
        state
            .pane
            .clone()
            .filter(|_| state.generation == generation)
    }

    fn lock(&self) -> MutexGuard<'_, OwnedPane> {
        self.state.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn assignment_is_shared_between_clones() {
        let owner = Owner::default();
        let reader = owner.clone();
        assert_eq!(reader.current(), None, "starts free");
        owner.assign(PaneId::new("p1").unwrap());
        assert_eq!(reader.current(), PaneId::new("p1").ok(), "assigned");
        owner.clear();
        assert_eq!(reader.current(), None, "cleared");
    }

    #[test]
    fn only_the_current_generation_resolves() {
        let owner = Owner::default();
        let first = owner.assign(PaneId::new("p1").unwrap());
        assert_eq!(owner.resolve(first), PaneId::new("p1").ok(), "current page");
        let second = owner.assign(PaneId::new("p2").unwrap());
        assert_eq!(owner.resolve(first), None, "the previous page is stale");
        assert_eq!(owner.resolve(second), PaneId::new("p2").ok(), "new page");
        owner.clear();
        assert_eq!(owner.resolve(second), None, "nobody owns it");
    }
}
