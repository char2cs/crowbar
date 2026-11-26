//! What the shell holds of its paired hosts besides their streams.

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
