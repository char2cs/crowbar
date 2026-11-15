use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use protocol::{HostId, PaneId, RelayScope};
use tokio_util::sync::CancellationToken;

use crate::connection_guard::ConnectionGuard;
use crate::connection_release::ConnectionRelease;
use crate::live::Live;
use crate::registry::Registry;

pub(crate) struct ConnectionBook {
    registry: Mutex<Registry>,
    max_total: usize,
    max_per_pane: usize,
}

impl ConnectionBook {
    pub(crate) fn new(
        max_total: usize,
        max_per_pane: usize,
    ) -> Arc<Self> {
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
            .filter(|l| l.pane == *scope.pane())
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
                pane: scope.pane().clone(),
                host: scope.host().clone(),
                cancel: cancel.clone(),
            },
        );
        Some(ConnectionGuard::new(
            Arc::clone(self) as Arc<dyn ConnectionRelease>,
            id,
            cancel,
        ))
    }

    pub(crate) fn close_pane(
        &self,
        pane: &PaneId,
    ) {
        self.close_where(|live| live.pane == *pane);
    }

    pub(crate) fn close_host(
        &self,
        host: &HostId,
    ) {
        self.close_where(|live| live.host == *host);
    }

    pub(crate) fn close_hosts_not_kept(
        &self,
        keep: impl Fn(&HostId) -> bool,
    ) {
        self.close_where(|live| !keep(&live.host));
    }

    fn close_where(
        &self,
        matches: impl Fn(&Live) -> bool,
    ) {
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

impl ConnectionRelease for ConnectionBook {
    fn release(
        &self,
        id: u64,
    ) {
        self.lock().live.remove(&id);
    }
}

#[cfg(test)]
mod tests {
    use protocol::PaneOrigin;

    use super::*;

    fn scope(
        pane: &str,
        host: &str,
    ) -> RelayScope {
        RelayScope::new(
            HostId::new(host).unwrap(),
            PaneId::new(pane).unwrap(),
            PaneOrigin::Core,
        )
    }

    #[test]
    fn the_total_and_per_pane_limits_hold_and_free_up_on_drop() {
        let book = ConnectionBook::new(3, 2);
        let root = CancellationToken::new();
        let first = book.register(&scope("p1", "local"), &root).unwrap();
        let _second = book.register(&scope("p1", "local"), &root).unwrap();
        assert!(
            book.register(&scope("p1", "local"), &root).is_none(),
            "per pane"
        );
        let _third = book.register(&scope("p2", "local"), &root).unwrap();
        assert!(
            book.register(&scope("p3", "local"), &root).is_none(),
            "total"
        );
        drop(first);
        assert!(book.register(&scope("p3", "local"), &root).is_some());
    }

    #[test]
    fn closing_a_pane_or_a_host_cancels_only_theirs() {
        let book = ConnectionBook::new(10, 10);
        let root = CancellationToken::new();
        let a = book.register(&scope("p1", "local"), &root).unwrap();
        let b = book.register(&scope("p2", "host-1"), &root).unwrap();
        let c = book.register(&scope("p3", "local"), &root).unwrap();
        book.close_pane(&PaneId::new("p1").unwrap());
        book.close_host(&HostId::new("host-1").unwrap());
        assert!(a.cancelled().is_cancelled());
        assert!(b.cancelled().is_cancelled());
        assert!(!c.cancelled().is_cancelled());
        assert_eq!(book.lock().live.len(), 3);
    }

    #[test]
    fn shutting_the_root_down_cancels_every_connection() {
        let book = ConnectionBook::new(10, 10);
        let root = CancellationToken::new();
        let a = book.register(&scope("p1", "local"), &root).unwrap();
        root.cancel();
        assert!(a.cancelled().is_cancelled());
    }

    #[test]
    fn closing_hosts_not_kept_cancels_every_stream_of_an_unlisted_host() {
        let book = ConnectionBook::new(10, 10);
        let root = CancellationToken::new();
        let local = book.register(&scope("p1", "local"), &root).unwrap();
        let listed = book.register(&scope("p2", "host-1"), &root).unwrap();
        let removed = book.register(&scope("p3", "host-2"), &root).unwrap();
        book.close_hosts_not_kept(|host| host.as_str() != "host-2");
        assert!(!local.cancelled().is_cancelled());
        assert!(!listed.cancelled().is_cancelled());
        assert!(removed.cancelled().is_cancelled());
    }
}
