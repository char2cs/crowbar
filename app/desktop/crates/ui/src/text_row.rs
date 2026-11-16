//! One visual row of a wrapped text.

use std::ops::Range;

use gpui::Pixels;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct TextRow {
    pub(crate) bytes: Range<usize>,
    pub(crate) caret_end: usize,
    pub(crate) top: Pixels,
    pub(crate) origin: Pixels,
    pub(crate) line: usize,
    pub(crate) closes_line: bool,
}

impl TextRow {
    pub(crate) fn holds(
        &self,
        index: usize,
    ) -> bool {
        index < self.bytes.end || (self.closes_line && index <= self.bytes.end)
    }
}

#[cfg(test)]
mod tests {
    use gpui::px;

    use super::*;
    use crate::pose::Pose;

    struct Case {
        row: TextRow,
        index: usize,
        holds: bool,
    }

    fn case(
        bytes: Range<usize>,
        closes_line: Pose,
        (index, holds): (usize, bool),
    ) -> Case {
        let row = TextRow {
            caret_end: bytes.end,
            bytes,
            top: px(0.0),
            origin: px(0.0),
            line: 0,
            closes_line: closes_line == Pose::On,
        };
        Case { row, index, holds }
    }

    #[test]
    fn a_wrap_point_belongs_to_the_row_it_starts() {
        let cases = [
            case(0..5, Pose::Off, (4, true)),
            case(0..5, Pose::Off, (5, false)),
            case(0..5, Pose::On, (5, true)),
            case(0..5, Pose::On, (6, false)),
            case(3..3, Pose::On, (3, true)),
        ];
        for case in cases {
            assert_eq!(
                case.row.holds(case.index),
                case.holds,
                "{:?} {}",
                case.row,
                case.index
            );
        }
    }
}
