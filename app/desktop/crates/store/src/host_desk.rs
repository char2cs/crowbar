//! What the shell holds of its paired hosts besides their streams.

use std::sync::Arc;

use crate::host_book::HostBook;
use crate::host_notes::HostNotes;

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
