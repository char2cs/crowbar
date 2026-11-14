use std::sync::Arc;

use tokio_util::sync::CancellationToken;

use crate::connection_book::ConnectionBook;

pub(crate) struct ConnectionGuard {
    book: Arc<ConnectionBook>,
    id: u64,
    cancel: CancellationToken,
}

impl ConnectionGuard {
    pub(crate) fn new(book: Arc<ConnectionBook>, id: u64, cancel: CancellationToken) -> Self {
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
#[path = "connection_guard_tests.rs"]
mod tests;
