//! The numbers the frame is laid out from.

use crate::dock_side::DockSide;
use crate::panel::Panel;
use crate::presentation::Presentation;
use crate::visibility::Visibility;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct FrameGeometry {
    pub(crate) container: f32,
    pub(crate) presentation: Presentation,
    pub(crate) visibility: Visibility,
    pub(crate) width: f32,
    pub(crate) side: DockSide,
}

impl FrameGeometry {
    pub(crate) fn new(container: f32, width: f32, panel: Panel) -> Self {
        Self {
            container,
            presentation: Panel::presentation(container),
            visibility: panel.visibility(),
            width,
            side: panel.side(),
        }
    }

    pub(crate) fn is_docked(&self) -> bool {
        self.visibility == Visibility::Open
            && self.presentation == Presentation::Docked
            && self.width > 0.0
    }

    pub(crate) fn is_drawer(&self) -> bool {
        self.visibility == Visibility::Open && self.presentation == Presentation::Overlay
    }
}

#[cfg(test)]
#[path = "frame_geometry_tests.rs"]
mod tests;
