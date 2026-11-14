//! The modal's access to record lists.

use crate::error::Error;
use crate::record_command::RecordCommand;
use crate::records::Records;

/// Where agents, active sessions and hosts come from.
pub trait RecordBackend {
    /// The current content of the list `source`.
    fn records(&self, source: &str) -> Records;

    /// Applies `command` to the record `record` of the list `source`.
    ///
    /// # Errors
    /// [`Error::Unavailable`] when the owner of the list cannot be reached.
    fn apply(&self, source: &str, record: &str, command: RecordCommand) -> Result<(), Error>;
}

#[cfg(test)]
#[path = "record_backend_tests.rs"]
mod tests;
