//! A create, reparent, detach or retry operation shown in place of a row.

use crate::operation_phase::OperationPhase;

/// An in-flight or failed daemon operation.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Operation {
    /// The label: "New thread", a branch name or "New workspace".
    pub label: String,
    /// Running or failed.
    pub phase: OperationPhase,
}
