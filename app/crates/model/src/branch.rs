use serde::{Deserialize, Serialize};

/// Which child of a split a path step enters.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum Branch {
    /// The first child (left or top).
    First,
    /// The second child (right or bottom).
    Second,
}
