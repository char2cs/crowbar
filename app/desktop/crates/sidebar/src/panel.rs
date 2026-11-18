//! Width, visibility and dock side of the sidebar column.

use crate::dock_side::DockSide;
use crate::metrics::{
    COLLAPSE_BELOW, DEFAULT_WIDTH, MAX_WIDTH, MIN_WIDTH, NARROW_BELOW, WINDOW_SHARE,
};
use crate::presentation::Presentation;
use crate::visibility::Visibility;

/// The sidebar column: open or closed, its remembered width and the edge it docks to.
/// Window pressure narrows the drawn track but never the remembered width.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Panel {
    visibility: Visibility,
    preferred: f32,
    side: DockSide,
}

impl Default for Panel {
    fn default() -> Self {
        Self {
            visibility: Visibility::Open,
            preferred: DEFAULT_WIDTH,
            side: DockSide::Left,
        }
    }
}

impl Panel {
    /// A panel from stored preferences, with the width clamped to the allowed range.
    pub fn restore(
        visibility: Visibility,
        preferred: f32,
        side: DockSide,
    ) -> Self {
        Self {
            visibility,
            preferred: preferred.clamp(MIN_WIDTH, MAX_WIDTH),
            side,
        }
    }

    /// Open or closed.
    pub fn visibility(self) -> Visibility {
        self.visibility
    }

    /// The remembered width, used whenever the window has room for it.
    pub fn preferred(self) -> f32 {
        self.preferred
    }

    /// The docked edge.
    pub fn side(self) -> DockSide {
        self.side
    }

    /// Docks the sidebar to `side`, keeping its width.
    pub fn dock_to(
        &mut self,
        side: DockSide,
    ) {
        self.side = side;
    }

    /// Opens a closed sidebar or closes an open one.
    pub fn toggle(&mut self) {
        self.visibility = self.visibility.flipped();
    }

    pub(crate) fn max_width(container: f32) -> f32 {
        MAX_WIDTH
            .min((container - 1.0) * WINDOW_SHARE)
            .max(MIN_WIDTH)
    }

    /// The width the column takes in a window `container` wide: 0 when closed, otherwise the
    /// remembered width capped at the window's share.
    pub fn track_width(
        self,
        container: f32,
    ) -> f32 {
        match self.visibility {
            Visibility::Closed => 0.0,
            Visibility::Open => self.preferred.min(container * WINDOW_SHARE),
        }
    }

    pub(crate) fn settle(
        width: f32,
        container: f32,
    ) -> f32 {
        if width < COLLAPSE_BELOW {
            return 0.0;
        }
        width.clamp(MIN_WIDTH, Self::max_width(container))
    }

    /// Keeps the width a resize ended on: 0 or less closes the sidebar and keeps the old
    /// width; any other width opens it and is remembered, rounded and clamped.
    pub fn commit(
        &mut self,
        width: f32,
    ) {
        if width <= 0.0 {
            self.visibility = Visibility::Closed;
            return;
        }
        self.visibility = Visibility::Open;
        self.preferred = width.round().clamp(MIN_WIDTH, MAX_WIDTH);
    }

    /// How an open sidebar sits in a window `container` wide: over the content when the
    /// window is narrow, docked beside it otherwise.
    pub fn presentation(container: f32) -> Presentation {
        if container < NARROW_BELOW {
            return Presentation::Overlay;
        }
        Presentation::Docked
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn assert_close(
        actual: f32,
        expected: f32,
        what: &str,
    ) {
        assert!(
            (actual - expected).abs() < 0.01,
            "{what}: {actual} != {expected}"
        );
    }

    #[test]
    fn restore_clamps_the_stored_width() {
        for (stored, expected) in [(100.0, 250.0), (294.0, 294.0), (900.0, 640.0)] {
            let panel = Panel::restore(Visibility::Open, stored, DockSide::Left);
            assert_close(panel.preferred(), expected, "restored preference");
        }
    }

    #[test]
    fn closed_track_has_no_width_and_window_pressure_does_not_touch_the_preference() {
        let mut panel = Panel::default();
        assert_close(panel.track_width(1200.0), 294.0, "open track");
        assert_close(panel.track_width(300.0), 240.0, "squeezed to 80%");
        assert_close(panel.preferred(), 294.0, "preference survives pressure");
        panel.toggle();
        assert_close(panel.track_width(1200.0), 0.0, "closed track");
    }

    #[test]
    fn settle_collapses_below_half_the_minimum_and_clamps_otherwise() {
        let cases = [
            (124.0, 1200.0, 0.0),
            (125.0, 1200.0, 250.0),
            (400.0, 1200.0, 400.0),
            (900.0, 1200.0, 640.0),
            (900.0, 700.0, 559.2),
        ];
        for (width, container, expected) in cases {
            assert_close(Panel::settle(width, container), expected, "settled width");
        }
    }

    #[test]
    fn commit_zero_closes_and_keeps_the_old_preference() {
        let mut panel = Panel::default();
        panel.commit(0.0);
        assert_eq!(panel.visibility(), Visibility::Closed, "zero closes");
        assert_close(panel.preferred(), 294.0, "preference kept");
        panel.commit(420.4);
        assert_eq!(panel.visibility(), Visibility::Open, "a width reopens");
        assert_close(panel.preferred(), 420.0, "rounded preference");
    }

    #[test]
    fn narrow_windows_use_the_drawer() {
        assert_eq!(Panel::presentation(639.0), Presentation::Overlay, "narrow");
        assert_eq!(Panel::presentation(640.0), Presentation::Docked, "wide");
    }

    #[test]
    fn dock_side_changes_without_touching_width() {
        let mut panel = Panel::default();
        panel.dock_to(DockSide::Right);
        assert_eq!(panel.side(), DockSide::Right, "side");
        assert_close(panel.preferred(), 294.0, "width");
    }
}
