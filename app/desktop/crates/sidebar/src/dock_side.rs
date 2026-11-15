//! Which window edge the sidebar docks to.

use serde::{Deserialize, Serialize};

const LEFT_SIGN: f32 = 1.0;
const RIGHT_SIGN: f32 = -1.0;

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
            Self::Left => LEFT_SIGN,
            Self::Right => RIGHT_SIGN,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn growth_follows_the_dock_edge() {
        assert!(
            (DockSide::Left.growth_sign() - 1.0).abs() < f32::EPSILON,
            "left grows rightwards"
        );
        assert!(
            (DockSide::Right.growth_sign() + 1.0).abs() < f32::EPSILON,
            "right grows leftwards"
        );
    }

    #[test]
    fn default_dock_is_left() {
        assert_eq!(
            DockSide::default(),
            DockSide::Left,
            "sidebarPosition defaults to left"
        );
    }
}
