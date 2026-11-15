//! Geometry of the scrollbar thumb.

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
        let total = (lines + history) as f32;
        let length = (track * lines as f32 / total).max(MIN_THUMB).min(track);
        let from_top = history.saturating_sub(display_offset) as f32;
        let top = (track - length) * from_top / history as f32;
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
        let from_top = (fraction * history as f32).round() as usize;
        history.saturating_sub(from_top)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const TRACK: f32 = 100.0;
    const LINES: usize = 10;
    const HISTORY: usize = 90;

    type Case = (usize, usize, Option<f32>);

    #[test]
    fn thumb_position_follows_the_scroll_offset() {
        const CASES: [Case; 3] = [
            (0, 0, None),
            (HISTORY, 0, Some(76.0)),
            (HISTORY, HISTORY, Some(0.0)),
        ];
        for (history, offset, expected_top) in CASES {
            let thumb = ScrollbarThumb::new(TRACK, LINES, history, offset);
            let top = thumb.map(|thumb| thumb.top);
            match (top, expected_top) {
                (Some(actual), Some(wanted)) => assert!((actual - wanted).abs() < 0.01, "{offset}"),
                (actual, wanted) => assert_eq!(actual.is_some(), wanted.is_some(), "{offset}"),
            }
        }
    }

    #[test]
    fn dragging_maps_back_to_an_offset() {
        const LENGTH: f32 = 24.0;
        const CASES: [(f32, usize); 3] = [(0.0, HISTORY), (38.0, 45), (1000.0, 0)];
        for (top, expected) in CASES {
            assert_eq!(
                ScrollbarThumb::offset_at(TRACK, LENGTH, HISTORY, top),
                expected,
                "{top}"
            );
        }
    }
}
