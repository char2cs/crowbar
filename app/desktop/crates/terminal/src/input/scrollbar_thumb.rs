//! Geometry of the scrollbar thumb.

use crate::input::geometry::{cells_f32, floor_cells};

const MIN_THUMB: f32 = 24.0;

/// The thumb's offset and length along a track, in pixels.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct ScrollbarThumb {
    /// Distance from the top of the track.
    pub top: f32,
    /// Thumb length.
    pub length: f32,
}

impl ScrollbarThumb {
    /// The thumb for a view `display_offset` lines above the bottom of `history` lines of scrollback
    /// plus `lines` visible ones, or `None` when everything fits (no history).
    pub fn new(
        track: f32,
        lines: usize,
        history: usize,
        display_offset: usize,
    ) -> Option<Self> {
        if history == 0 || track <= 0.0 {
            return None;
        }
        let total = cells_f32(lines + history);
        let length = (track * cells_f32(lines) / total).max(MIN_THUMB).min(track);
        let from_top = cells_f32(history.saturating_sub(display_offset));
        let top = (track - length) * from_top / cells_f32(history);
        Some(Self { top, length })
    }

    /// The display offset a thumb whose top is at `top` stands for.
    pub fn offset_at(
        track: f32,
        length: f32,
        history: usize,
        top: f32,
    ) -> usize {
        let travel = (track - length).max(f32::EPSILON);
        let fraction = (top / travel).clamp(0.0, 1.0);
        let from_top = floor_cells((fraction * cells_f32(history)).round());
        history.saturating_sub(from_top)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const TRACK: f32 = 100.0;
    const HISTORY: usize = 90;

    #[test]
    fn the_thumb_follows_the_scroll_offset_and_maps_back() {
        let tops = [
            (0, 0, None),
            (HISTORY, 0, Some(76.0)),
            (HISTORY, HISTORY, Some(0.0)),
        ];
        for (history, offset, expected_top) in tops {
            let top = ScrollbarThumb::new(TRACK, 10, history, offset).map(|thumb| thumb.top);
            let close = match (top, expected_top) {
                (Some(actual), Some(wanted)) => (actual - wanted).abs() < 0.01,
                (actual, wanted) => actual.is_some() == wanted.is_some(),
            };
            assert!(close, "{offset}: {top:?}");
        }
        let offsets = [(0.0, HISTORY), (38.0, 45), (1000.0, 0)];
        for (top, expected) in offsets {
            let offset = ScrollbarThumb::offset_at(TRACK, MIN_THUMB, HISTORY, top);
            assert_eq!(offset, expected, "{top}");
        }
    }
}
