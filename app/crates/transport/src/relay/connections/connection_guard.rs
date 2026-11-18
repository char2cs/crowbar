use std::sync::Arc;

use tokio_util::sync::CancellationToken;

use crate::relay::connections::connection_release::ConnectionRelease;

pub(crate) struct ConnectionGuard {
    book: Arc<dyn ConnectionRelease>,
    id: u64,
    cancel: CancellationToken,
}

impl ConnectionGuard {
    pub(crate) fn new(
        book: Arc<dyn ConnectionRelease>,
        id: u64,
        cancel: CancellationToken,
    ) -> Self {
        Self { book, id, cancel }
    }

    pub(crate) fn cancelled(&self) -> CancellationToken {
        self.cancel.clone()
    }
}

impl Drop for ConnectionGuard {
    fn drop(&mut self) {
        self.book.release(self.id);
    }
}

#[cfg(test)]
mod tests {
    use protocol::{HostId, PaneId, PaneOrigin, RelayScope};

    use super::*;
    use crate::relay::connections::connection_book::ConnectionBook;

    fn scope() -> RelayScope {
        RelayScope::new(
            HostId::new("local").unwrap(),
            PaneId::new("p1").unwrap(),
            PaneOrigin::Core,
        )
    }

    #[test]
    fn dropping_the_guard_frees_its_slot() {
        let book = ConnectionBook::new(1, 1);
        let root = CancellationToken::new();
        let guard = book.register(&scope(), &root).unwrap();
        assert!(book.register(&scope(), &root).is_none());
        drop(guard);
        assert!(book.register(&scope(), &root).is_some());
    }

    #[test]
    fn the_guard_hands_out_the_cancellation_of_its_own_stream() {
        let book = ConnectionBook::new(2, 2);
        let root = CancellationToken::new();
        let guard = book.register(&scope(), &root).unwrap();
        let cancelled = guard.cancelled();
        assert!(!cancelled.is_cancelled());
        book.close_pane(&PaneId::new("p1").unwrap());
        assert!(cancelled.is_cancelled());
    }
}
