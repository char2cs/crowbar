//! Where the pointer summons and keeps the peek card.

use crate::dock_side::DockSide;
use crate::metrics::{HEADER_HEIGHT, PEEK_INSET, PEEK_TRIGGER};

const BOTTOM_CLEAR: f32 = 16.0;
const SLACK: f32 = 8.0;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct PeekZone {
    pub(crate) window: (f32, f32),
    pub(crate) card_width: f32,
    pub(crate) side: DockSide,
}

impl PeekZone {
    fn distance_from_edge(&self, x: f32) -> f32 {
        match self.side {
            DockSide::Left => x,
            DockSide::Right => self.window.0 - x,
        }
    }

    pub(crate) fn summons(&self, x: f32, y: f32) -> bool {
        self.distance_from_edge(x) <= PEEK_TRIGGER
            && y >= HEADER_HEIGHT
            && y <= self.window.1 - BOTTOM_CLEAR
    }

    pub(crate) fn keeps(&self, x: f32, _y: f32) -> bool {
        self.distance_from_edge(x) <= PEEK_INSET + self.card_width + SLACK
    }
}

#[cfg(test)]
#[path = "peek_zone_tests.rs"]
mod tests;
