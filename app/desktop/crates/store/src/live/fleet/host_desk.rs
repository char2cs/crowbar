//! What the shell holds of its paired hosts besides their streams.

mod book_error;
mod host_book;
mod host_note;
mod host_notes;
mod host_roster;
mod pairing_request;

pub use book_error::BookError;
pub use host_book::{BookFuture, HostBook};
pub use host_note::HostNote;
pub use host_notes::HostNotes;
pub(crate) use host_roster::HostRoster;
pub use pairing_request::PairingRequest;

use std::sync::Arc;

/// The transport's host book and the shell's notes on each host.
#[derive(Clone)]
pub struct HostDesk {
    /// Pairs and forgets hosts.
    pub book: Arc<dyn HostBook>,
    /// Names, client ids and fingerprints of the paired hosts.
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
