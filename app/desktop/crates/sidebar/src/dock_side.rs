//! Which window edge the sidebar docks to.

use serde::{Deserialize, Serialize};

/// The edge of the window the sidebar sits against; `Left` is the default.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
pub enum DockSide {
    /// Against the left edge.
    #[default]
    Left,
    /// Against the right edge.
    Right,
}

impl DockSide {
    /// `+1.0` when growing the sidebar means moving the pointer right, `-1.0` when it means left.
    pub fn growth_sign(self) -> f32 {
        match self {
            Self::Left => 1.0,
            Self::Right => -1.0,
        }
    }
}

#[cfg(test)]
#[path = "dock_side_tests.rs"]
mod tests;
