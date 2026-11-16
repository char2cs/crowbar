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
    pub(crate) fn docked_toasts(self) -> ui::ToastPlacement {
        ui::ToastPlacement::Docked(self.toast_side())
    }

    /// The toast edge matching this dock edge: toasts swipe away toward it.
    pub fn toast_side(self) -> ui::ToastSide {
        match self {
            Self::Left => ui::ToastSide::Left,
            Self::Right => ui::ToastSide::Right,
        }
    }

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
    fn each_edge_grows_and_swipes_toward_itself() {
        const CASES: [(DockSide, f32, ui::ToastSide); 2] = [
            (DockSide::Left, 1.0, ui::ToastSide::Left),
            (DockSide::Right, -1.0, ui::ToastSide::Right),
        ];
        for (side, sign, toast) in CASES {
            assert!((side.growth_sign() - sign).abs() < f32::EPSILON, "{side:?}");
            assert_eq!(side.toast_side(), toast, "{side:?}");
        }
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
