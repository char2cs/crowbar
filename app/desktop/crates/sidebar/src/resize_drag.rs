//! One pointer drag of the sidebar separator.

use crate::dock_side::DockSide;
use crate::panel::Panel;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct ResizeDrag {
    start_width: f32,
    press_x: f32,
    side: DockSide,
    container: f32,
}

impl ResizeDrag {
    pub(crate) fn begin(
        start_width: f32,
        press_x: f32,
        panel: Panel,
        container: f32,
    ) -> Self {
        Self {
            start_width,
            press_x,
            side: panel.side(),
            container,
        }
    }

    pub(crate) fn width_at(
        &self,
        pointer_x: f32,
    ) -> f32 {
        let delta = (pointer_x - self.press_x) * self.side.growth_sign();
        Panel::settle(self.start_width + delta, self.container)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn drag(side: DockSide) -> ResizeDrag {
        let mut panel = Panel::default();
        panel.dock_to(side);
        ResizeDrag::begin(294.0, 294.0, panel, 1200.0)
    }

    type WidthCase = (&'static str, DockSide, f32, f32);

    const WIDTH_CASES: &[WidthCase] = &[
        (
            "dragging right grows a left dock",
            DockSide::Left,
            420.0,
            420.0,
        ),
        (
            "dragging left grows a right dock",
            DockSide::Right,
            168.0,
            420.0,
        ),
        ("clamped to the minimum", DockSide::Left, 200.0, 250.0),
        (
            "collapsed below the collapse point",
            DockSide::Left,
            100.0,
            0.0,
        ),
    ];

    #[test]
    fn the_pointer_sets_the_width_mirrored_for_the_dock_side() {
        for (name, side, pointer, expected) in WIDTH_CASES {
            let width = drag(*side).width_at(*pointer);
            assert!((width - expected).abs() < 0.01, "{name}: got {width}");
        }
    }

    #[test]
    fn the_drag_reopens_when_the_pointer_returns_from_the_collapse_point() {
        let drag = drag(DockSide::Left);
        assert!(drag.width_at(100.0).abs() < 0.01, "collapsed");
        assert!((drag.width_at(200.0) - 250.0).abs() < 0.01, "reopened");
    }
}
