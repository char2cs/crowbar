//! Where the pointer summons and keeps the peek card.

use crate::dock_side::DockSide;
use crate::metrics::{HEADER_HEIGHT, PEEK_INSET, PEEK_TRIGGER};

const BOTTOM_CLEAR: f32 = 16.0;
const SLACK: f32 = 8.0;

/// The window-edge band that summons the peek card while the sidebar is hidden, and the wider
/// area that keeps it open.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct PeekZone {
    /// The window's width and height in pixels.
    pub window: (f32, f32),
    /// The peek card's width in pixels.
    pub card_width: f32,
    /// The window edge the sidebar docks to.
    pub side: DockSide,
}

impl PeekZone {
    fn distance_from_edge(
        &self,
        x: f32,
    ) -> f32 {
        match self.side {
            DockSide::Left => x,
            DockSide::Right => self.window.0 - x,
        }
    }

    /// Whether a pointer at (`x`, `y`) summons the card: within a few pixels of the docked edge,
    /// below the header and clear of the bottom corner.
    pub fn summons(
        &self,
        x: f32,
        y: f32,
    ) -> bool {
        self.distance_from_edge(x) <= PEEK_TRIGGER
            && y >= HEADER_HEIGHT
            && y <= self.window.1 - BOTTOM_CLEAR
    }

    /// Whether a pointer at `x` keeps an open card: over the card or within eight pixels of its
    /// far edge. The height is ignored.
    pub fn keeps(
        &self,
        x: f32,
        _y: f32,
    ) -> bool {
        self.distance_from_edge(x) <= PEEK_INSET + self.card_width + SLACK
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn zone(side: DockSide) -> PeekZone {
        PeekZone {
            window: (1200.0, 800.0),
            card_width: 294.0,
            side,
        }
    }

    type SummonCase = (&'static str, DockSide, f32, f32, bool);

    const SUMMON_CASES: &[SummonCase] = &[
        ("top of the band", DockSide::Left, 0.0, 42.0, true),
        ("bottom of the band", DockSide::Left, 6.0, 784.0, true),
        ("chrome row", DockSide::Left, 0.0, 41.0, false),
        ("bottom corner", DockSide::Left, 0.0, 785.0, false),
        ("outside the band", DockSide::Left, 7.0, 400.0, false),
        ("right dock inside", DockSide::Right, 1194.0, 400.0, true),
        ("right dock outside", DockSide::Right, 1100.0, 400.0, false),
    ];

    #[test]
    fn the_band_starts_below_the_chrome_row_and_mirrors_for_the_right_dock() {
        for (name, side, x, y, expected) in SUMMON_CASES {
            assert_eq!(zone(*side).summons(*x, *y), *expected, "{name}");
        }
    }

    const KEEP_CASES: &[(&str, f32, bool)] = &[
        ("inside the slack", 310.0, true),
        ("past the slack", 311.0, false),
    ];

    #[test]
    fn the_card_keeps_the_pointer_within_eight_pixels_of_its_edge() {
        for (name, x, expected) in KEEP_CASES {
            assert_eq!(zone(DockSide::Left).keeps(*x, 400.0), *expected, "{name}");
        }
    }
}
