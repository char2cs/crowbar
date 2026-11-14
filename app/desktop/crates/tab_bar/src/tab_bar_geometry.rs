use gpui::{Pixels, px};

use crate::sidebar_state::SidebarState;
use crate::span::Span;
use crate::strip_metrics::{ROW_GAP, ROW_PADDING, TOGGLE_SIZE, slots};
use crate::tab_bar::TabBar;

impl TabBar {
    pub(crate) fn strip_left(&self) -> Pixels {
        let toggle = match self.sidebar {
            SidebarState::ClosedLeft => TOGGLE_SIZE + ROW_GAP,
            SidebarState::Open | SidebarState::ClosedRight => 0.0,
        };
        self.row.origin.x + px(ROW_PADDING + toggle)
    }

    pub(crate) fn strip_width(&self) -> f32 {
        let toggle = match self.sidebar {
            SidebarState::Open => 0.0,
            SidebarState::ClosedLeft | SidebarState::ClosedRight => TOGGLE_SIZE + ROW_GAP,
        };
        (f32::from(self.row.size.width) - 2.0 * ROW_PADDING - toggle).max(0.0)
    }

    pub(crate) fn slot_spans(&self) -> Vec<Span> {
        slots(self.tabs.len(), self.strip_width())
    }
}

#[cfg(test)]
#[path = "tab_bar_geometry_tests.rs"]
mod tests;
