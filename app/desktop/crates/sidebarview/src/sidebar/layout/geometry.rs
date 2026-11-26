//! The numbers the frame is laid out from.

use sidebar::{DockSide, Panel, Presentation, Visibility};

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct Geometry {
    pub(crate) container: f32,
    pub(crate) presentation: Presentation,
    pub(crate) visibility: Visibility,
    pub(crate) width: f32,
    pub(crate) side: DockSide,
}

impl Geometry {
    pub(crate) fn new(
        container: f32,
        width: f32,
        panel: Panel,
    ) -> Self {
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
mod tests {
    use super::*;

    fn panel(visibility: Visibility) -> Panel {
        Panel::restore(visibility, 294.0, DockSide::Left)
    }

    type Case = (&'static str, f32, f32, Visibility, bool, bool);

    const CASES: &[Case] = &[
        (
            "open in a wide window docks",
            1200.0,
            294.0,
            Visibility::Open,
            true,
            false,
        ),
        (
            "open in a narrow window is a drawer",
            600.0,
            294.0,
            Visibility::Open,
            false,
            true,
        ),
        (
            "closed is neither",
            1200.0,
            0.0,
            Visibility::Closed,
            false,
            false,
        ),
        (
            "zero width never docks",
            1200.0,
            0.0,
            Visibility::Open,
            false,
            false,
        ),
    ];

    #[test]
    fn the_window_and_panel_choose_dock_or_drawer() {
        for (name, window, width, visibility, docked, drawer) in CASES {
            let geometry = Geometry::new(*window, *width, panel(*visibility));
            assert_eq!(geometry.is_docked(), *docked, "{name}: docked");
            assert_eq!(geometry.is_drawer(), *drawer, "{name}: drawer");
        }
    }
}
