//! One pointer drag of the sidebar separator.

use crate::dock_side::DockSide;
use crate::panel::Panel;

/// The state of a resize between pointer press and release; owns the arithmetic, not the pointer.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct ResizeDrag {
    start_width: f32,
    press_x: f32,
    side: DockSide,
    container: f32,
}

impl ResizeDrag {
    /// Starts a drag at `press_x` while the column is `start_width` wide inside `container`.
    pub fn begin(start_width: f32, press_x: f32, panel: &Panel, container: f32) -> Self {
        Self {
            start_width,
            press_x,
            side: panel.side(),
            container,
        }
    }

    /// The live width for a pointer at `pointer_x`; zero means collapsed.
    pub fn width_at(&self, pointer_x: f32) -> f32 {
        let delta = (pointer_x - self.press_x) * self.side.growth_sign();
        Panel::settle(self.start_width + delta, self.container)
    }

    /// The width to restore when the drag is cancelled by `pointercancel` or a window blur.
    pub fn cancelled_width(&self) -> f32 {
        self.start_width
    }
}

#[cfg(test)]
#[path = "resize_drag_tests.rs"]
mod tests;
