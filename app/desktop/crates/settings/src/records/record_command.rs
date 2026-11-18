//! What the user asks of a record.

/// A change to a record list that the backend owns.
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum RecordCommand {
    /// Turns a flag of the record on or off.
    SetFlag {
        /// The flag id.
        flag: String,
        /// The requested value.
        on: bool,
    },
    /// Moves the record to a position in the list.
    Move {
        /// The new index.
        to: usize,
    },
    /// Runs the record's action.
    Run {
        /// The action id.
        action: String,
    },
    /// Asks the backend to load the list again.
    Refresh,
}
