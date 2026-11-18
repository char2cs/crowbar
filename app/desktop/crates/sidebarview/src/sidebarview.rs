//! Where the toasts of the sidebar sit: the free functions of the crate root.

use sidebar::DockSide;
use ui::{ToastPlacement, ToastSide};

/// The toast edge matching the sidebar's dock edge: toasts swipe away toward it.
pub fn toast_side(side: DockSide) -> ToastSide {
    match side {
        DockSide::Left => ToastSide::Left,
        DockSide::Right => ToastSide::Right,
    }
}

pub(crate) fn docked_toasts(side: DockSide) -> ToastPlacement {
    ToastPlacement::Docked(toast_side(side))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_edge_swipes_toward_itself() {
        const CASES: [(DockSide, ToastSide); 2] = [
            (DockSide::Left, ToastSide::Left),
            (DockSide::Right, ToastSide::Right),
        ];
        for (side, toast) in CASES {
            assert_eq!(toast_side(side), toast, "{side:?}");
            assert_eq!(
                docked_toasts(side),
                ToastPlacement::Docked(toast),
                "{side:?}"
            );
        }
    }
}
