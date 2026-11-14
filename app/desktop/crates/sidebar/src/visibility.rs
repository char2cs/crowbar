//! Whether the sidebar column is shown.

use serde::{Deserialize, Serialize};

/// Open or closed; stands in for a `bool` where a function must not take one.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
pub enum Visibility {
    /// The sidebar is docked.
    #[default]
    Open,
    /// The sidebar column has no width.
    Closed,
}

impl Visibility {
    /// The other value.
    #[must_use]
    pub fn flipped(self) -> Self {
        match self {
            Self::Open => Self::Closed,
            Self::Closed => Self::Open,
        }
    }
}

#[cfg(test)]
#[path = "visibility_tests.rs"]
mod tests;
