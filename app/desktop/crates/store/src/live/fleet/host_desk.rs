mod book_error;
mod host_book;
mod host_notes;
mod note;
mod pairing_request;
mod roster;

pub use book_error::BookError;
pub use host_book::{BookFuture, HostBook};
pub use host_notes::HostNotes;
pub use note::Note;
pub use pairing_request::PairingRequest;
pub(crate) use roster::Roster;

use std::sync::Arc;

#[derive(Clone)]
pub struct HostDesk {
    pub book: Arc<dyn HostBook>,
    pub notes: Arc<HostNotes>,
}

impl std::fmt::Debug for HostDesk {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter.debug_struct("HostDesk").finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{FakeBook, notes};

    #[test]
    fn the_desk_debugs_without_its_parts() {
        let desk = HostDesk {
            book: Arc::new(FakeBook::default()),
            notes: Arc::new(notes()),
        };
        assert_eq!(format!("{desk:?}"), "HostDesk { .. }");
    }
}
