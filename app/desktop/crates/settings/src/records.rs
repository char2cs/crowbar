//! The state of a record list.

use crate::record::Record;

/// A list the backend is loading, failed to load, or holds.
#[derive(Debug, Clone, PartialEq)]
pub enum Records {
    /// The first answer has not arrived.
    Loading,
    /// The backend could not answer.
    Failed,
    /// The current records, possibly none.
    Ready(Vec<Record>),
}
