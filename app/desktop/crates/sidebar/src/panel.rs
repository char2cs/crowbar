//! Width, visibility and dock side of the sidebar column.

use crate::dock_side::DockSide;
use crate::metrics::{
    COLLAPSE_BELOW, DEFAULT_WIDTH, MAX_WIDTH, MIN_WIDTH, NARROW_BELOW, WINDOW_SHARE,
};
use crate::presentation::Presentation;
use crate::visibility::Visibility;

/// The remembered geometry of the sidebar; the preference is never rewritten by window pressure.
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
    /// A panel restored from stored values; the width is clamped to 250..=640.
    pub fn restore(visibility: Visibility, preferred: f32, side: DockSide) -> Self {
        Self {
            visibility,
            preferred: preferred.clamp(MIN_WIDTH, MAX_WIDTH),
            side,
        }
    }

    /// Whether the column is shown.
    pub fn visibility(&self) -> Visibility {
        self.visibility
    }

    /// The remembered width an open sidebar returns to.
    pub fn preferred(&self) -> f32 {
        self.preferred
    }

    /// The docked edge.
    pub fn side(&self) -> DockSide {
        self.side
    }

    /// Docks the sidebar to `side` without remounting anything.
    pub fn dock_to(&mut self, side: DockSide) {
        self.side = side;
    }

    /// Opens or closes the column.
    pub fn toggle(&mut self) {
        self.visibility = self.visibility.flipped();
    }

    /// The widest the column may grow in a container `container` pixels wide.
    pub fn max_width(container: f32) -> f32 {
        MAX_WIDTH
            .min((container - 1.0) * WINDOW_SHARE)
            .max(MIN_WIDTH)
    }

    /// The width of the grid track: zero while closed, the preference squeezed to 80% of the
    /// container otherwise.
    pub fn track_width(&self, container: f32) -> f32 {
        match self.visibility {
            Visibility::Closed => 0.0,
            Visibility::Open => self.preferred.min(container * WINDOW_SHARE),
        }
    }

    /// A width a pointer or key produced, made legal: below half the minimum it collapses to
    /// zero, anything else lands in `250..=max_width`.
    pub fn settle(width: f32, container: f32) -> f32 {
        if width < COLLAPSE_BELOW {
            return 0.0;
        }
        width.clamp(MIN_WIDTH, Self::max_width(container))
    }

    /// Stores the width a resize ended on: zero closes the column, anything else becomes the
    /// preference.
    pub fn commit(&mut self, width: f32) {
        if width <= 0.0 {
            self.visibility = Visibility::Closed;
            return;
        }
        self.visibility = Visibility::Open;
        self.preferred = width.round().clamp(MIN_WIDTH, MAX_WIDTH);
    }

    /// Docked in a wide window, a drawer in a narrow one.
    pub fn presentation(container: f32) -> Presentation {
        if container < NARROW_BELOW {
            return Presentation::Overlay;
        }
        Presentation::Docked
    }
}

#[cfg(test)]
#[path = "panel_tests.rs"]
mod tests;
