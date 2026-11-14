//! Whether a parent row shows its children.

use serde::{Deserialize, Serialize};

/// Folded parents hide their descendants.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
pub enum Expansion {
    /// Children are shown.
    #[default]
    Expanded,
    /// Children are hidden.
    Collapsed,
}
