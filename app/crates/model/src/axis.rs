use serde::{Deserialize, Serialize};

/// Direction in which a split arranges its two children.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum Axis {
    /// Children sit side by side, first on the left.
    Horizontal,
    /// Children are stacked, first on top.
    Vertical,
}
