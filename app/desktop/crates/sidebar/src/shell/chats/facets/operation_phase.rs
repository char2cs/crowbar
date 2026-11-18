//! Progress of an operation row.

/// An operation is running or has failed with a reason.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum OperationPhase {
    /// A flicker spinner shows.
    Running,
    /// The operation failed; the row offers Dismiss.
    Failed {
        /// Why it failed, truncated to half the row.
        reason: String,
    },
}
