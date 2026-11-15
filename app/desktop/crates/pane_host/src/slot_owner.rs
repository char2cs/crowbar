use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use protocol::PaneId;

#[derive(Debug, Clone, Default)]
pub(crate) struct SlotOwner {
    pane: Arc<Mutex<Option<PaneId>>>,
}

impl SlotOwner {
    pub(crate) fn assign(&self, pane: PaneId) {
        *self.lock() = Some(pane);
    }

    pub(crate) fn clear(&self) {
        *self.lock() = None;
    }

    pub(crate) fn current(&self) -> Option<PaneId> {
        self.lock().clone()
    }

    fn lock(&self) -> MutexGuard<'_, Option<PaneId>> {
        self.pane.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

#[cfg(test)]
#[path = "slot_owner_tests.rs"]
mod tests;
