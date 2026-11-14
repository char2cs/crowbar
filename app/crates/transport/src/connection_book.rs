use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use protocol::{HostId, PaneId, RelayScope};
use tokio_util::sync::CancellationToken;

use crate::connection_guard::ConnectionGuard;
use crate::live::Live;
use crate::registry::Registry;

pub(crate) struct ConnectionBook {
    registry: Mutex<Registry>,
    max_total: usize,
    max_per_pane: usize,
}

impl ConnectionBook {
    pub(crate) fn new(max_total: usize, max_per_pane: usize) -> Arc<Self> {
        Arc::new(Self {
            registry: Mutex::default(),
            max_total,
            max_per_pane,
        })
    }

    pub(crate) fn register(
        self: &Arc<Self>,
        scope: &RelayScope,
        parent: &CancellationToken,
    ) -> Option<ConnectionGuard> {
        let mut registry = self.lock();
        let same_pane = registry
            .live
            .values()
            .filter(|l| l.pane == scope.pane)
            .count();
        if registry.live.len() >= self.max_total || same_pane >= self.max_per_pane {
            return None;
        }
        let id = registry.next;
        registry.next += 1;
        let cancel = parent.child_token();
        registry.live.insert(
            id,
            Live {
                pane: scope.pane.clone(),
                host: scope.host.clone(),
                cancel: cancel.clone(),
            },
        );
        Some(ConnectionGuard::new(Arc::clone(self), id, cancel))
    }

    pub(crate) fn release(&self, id: u64) {
        self.lock().live.remove(&id);
    }

    pub(crate) fn close_pane(&self, pane: &PaneId) {
        self.close_where(|live| live.pane == *pane);
    }

    pub(crate) fn close_host(&self, host: &HostId) {
        self.close_where(|live| live.host == *host);
    }

    fn close_where(&self, matches: impl Fn(&Live) -> bool) {
        let registry = self.lock();
        registry
            .live
            .values()
            .filter(|live| matches(live))
            .for_each(|live| live.cancel.cancel());
    }

    fn lock(&self) -> MutexGuard<'_, Registry> {
        self.registry.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

#[cfg(test)]
#[path = "connection_book_tests.rs"]
mod tests;
