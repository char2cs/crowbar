//! Counts rapid clicks to choose between character, word and line selection.

use std::time::Duration;

use crate::input::geometry::ViewPoint;
use crate::input::pointer::selection_kind::SelectionKind;

const MULTI_CLICK_WINDOW: Duration = Duration::from_millis(500);
const WORD_CLICKS: u32 = 2;
const LINE_CLICKS: u32 = 3;

/// Remembers the last press so the next one can tell whether it continues a double or triple click.
#[derive(Debug, Clone, Default)]
pub struct ClickTracker {
    last: Option<(Duration, ViewPoint)>,
    count: u32,
}

impl ClickTracker {
    /// Records a press at `point` made `at` (any monotonic clock) and returns what it selects.
    pub fn press(
        &mut self,
        point: ViewPoint,
        at: Duration,
    ) -> SelectionKind {
        let next = self.continues(point, at).then(|| self.count + 1);
        self.count = next.unwrap_or(1);
        self.last = Some((at, point));
        match self.count {
            WORD_CLICKS => SelectionKind::Word,
            LINE_CLICKS => SelectionKind::Line,
            _ => SelectionKind::Simple,
        }
    }

    fn continues(
        &self,
        point: ViewPoint,
        at: Duration,
    ) -> bool {
        self.last.is_some_and(|(previous, place)| {
            place == point && at.saturating_sub(previous) <= MULTI_CLICK_WINDOW
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn quick_repeats_escalate_and_slow_or_moved_clicks_start_over() {
        let presses = [
            (0, 1, SelectionKind::Simple),
            (100, 1, SelectionKind::Word),
            (200, 1, SelectionKind::Line),
            (300, 1, SelectionKind::Simple),
            (1200, 1, SelectionKind::Simple),
            (1250, 1, SelectionKind::Word),
            (1260, 2, SelectionKind::Simple),
        ];
        let mut tracker = ClickTracker::default();
        for (millis, row, expected) in presses {
            let kind = tracker.press(ViewPoint { row, column: 1 }, Duration::from_millis(millis));
            assert_eq!(kind, expected, "{millis} ms");
        }
    }
}
