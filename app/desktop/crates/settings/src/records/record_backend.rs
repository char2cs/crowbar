//! The modal's access to record lists.

use crate::records::Records;
use crate::records::record_command::RecordCommand;
use crate::values::Error;

/// Where agents, active sessions and hosts come from.
pub trait RecordBackend {
    /// The current content of the list `source`.
    fn records(
        &self,
        source: &str,
    ) -> Records;

    /// Applies `command` to the record `record` of the list `source`.
    ///
    /// # Errors
    /// [`Error::Unavailable`] when the owner of the list cannot be reached.
    fn apply(
        &self,
        source: &str,
        record: &str,
        command: RecordCommand,
    ) -> Result<(), Error>;
}
