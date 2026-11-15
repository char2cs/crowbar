//! Counts rapid clicks to choose between character, word and line selection.

use std::time::Duration;

use crate::selection_kind::SelectionKind;
use crate::view_point::ViewPoint;

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

    fn at(
        row: usize,
        column: usize,
    ) -> ViewPoint {
        ViewPoint { row, column }
    }

    #[test]
    fn quick_repeats_escalate_then_start_over() {
        let mut tracker = ClickTracker::default();
        let kinds: Vec<SelectionKind> = (0..4)
            .map(|n| tracker.press(at(1, 1), Duration::from_millis(n * 100)))
            .collect();
        assert_eq!(
            kinds,
            [
                SelectionKind::Simple,
                SelectionKind::Word,
                SelectionKind::Line,
                SelectionKind::Simple
            ]
        );
    }

    #[test]
    fn a_slow_or_moved_click_is_a_fresh_click() {
        let mut tracker = ClickTracker::default();
        tracker.press(at(1, 1), Duration::ZERO);
        assert_eq!(
            tracker.press(at(1, 1), Duration::from_millis(900)),
            SelectionKind::Simple
        );
        tracker.press(at(1, 1), Duration::from_millis(950));
        assert_eq!(
            tracker.press(at(2, 1), Duration::from_millis(960)),
            SelectionKind::Simple
        );
    }
}
