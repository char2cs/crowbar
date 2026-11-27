use crate::frame::dock_side::DockSide;
use crate::frame::metrics::PEEK_TRIGGER;

const BOTTOM_CLEAR: f32 = 16.0;
const SLACK: f32 = 8.0;

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct PeekZone {
    pub window: (f32, f32),
    pub card_width: f32,
    pub side: DockSide,
    pub header_height: f32,
    pub card_inset: f32,
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

    pub fn summons(
        &self,
        x: f32,
        y: f32,
    ) -> bool {
        self.distance_from_edge(x) <= PEEK_TRIGGER
            && y >= self.header_height
            && y <= self.window.1 - BOTTOM_CLEAR
    }

    pub fn keeps(
        &self,
        x: f32,
        _y: f32,
    ) -> bool {
        self.distance_from_edge(x) <= self.keep_width()
    }

    pub fn keep_width(&self) -> f32 {
        self.card_inset + self.card_width + SLACK
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
            header_height: 42.0,
            card_inset: 8.0,
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
