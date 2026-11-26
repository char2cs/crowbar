use std::sync::Arc;

use tokio_util::sync::CancellationToken;

use crate::relay::connections::release::Release;

pub(crate) struct Guard {
    book: Arc<dyn Release>,
    id: u64,
    cancel: CancellationToken,
}

impl Guard {
    pub(crate) fn new(
        book: Arc<dyn Release>,
        id: u64,
        cancel: CancellationToken,
    ) -> Self {
        Self { book, id, cancel }
    }

    pub(crate) fn cancelled(&self) -> CancellationToken {
        self.cancel.clone()
    }
}

impl Drop for Guard {
    fn drop(&mut self) {
        self.book.release(self.id);
    }
}

#[cfg(test)]
mod tests {
    use protocol::{HostId, PaneId, PaneOrigin, RelayScope};

    use super::*;
    use crate::relay::connections::book::Book;

    fn scope() -> RelayScope {
        RelayScope::new(
            HostId::new("local").unwrap(),
            PaneId::new("p1").unwrap(),
            PaneOrigin::Core,
        )
    }

    #[test]
    fn dropping_the_guard_frees_its_slot() {
        let book = Book::new(1, 1);
        let root = CancellationToken::new();
        let guard = book.register(&scope(), &root).unwrap();
        assert!(book.register(&scope(), &root).is_none());
        drop(guard);
        assert!(book.register(&scope(), &root).is_some());
    }
}
