//! Maps grid lines to visible rows.

use alacritty_terminal::index::{Line, Point};

use crate::highlight::Highlight;

#[derive(Debug, Clone, Copy)]
pub(crate) struct Viewport {
    pub(crate) lines: usize,
    pub(crate) columns: usize,
    pub(crate) display_offset: usize,
}

impl Viewport {
    pub(crate) fn row_of(
        &self,
        line: Line,
    ) -> Option<usize> {
        let row = i64::from(line.0) + i64::try_from(self.display_offset).ok()?;
        usize::try_from(row).ok().filter(|row| *row < self.lines)
    }

    pub(crate) fn line_of(
        &self,
        row: usize,
    ) -> Line {
        let offset = i32::try_from(self.display_offset).unwrap_or(i32::MAX);
        let row = i32::try_from(row).unwrap_or(i32::MAX);
        Line(row.saturating_sub(offset))
    }

    pub(crate) fn highlights(
        &self,
        start: Point,
        end: Point,
    ) -> Vec<Highlight> {
        let offset = i64::try_from(self.display_offset).unwrap_or(0);
        let lines = i64::try_from(self.lines).unwrap_or(0);
        let first = i64::from(start.line.0).max(-offset);
        let last = i64::from(end.line.0).min(lines - offset - 1);
        (first..=last)
            .filter_map(|line| self.line_highlight(Line(i32::try_from(line).ok()?), start, end))
            .collect()
    }

    fn line_highlight(
        &self,
        line: Line,
        start: Point,
        end: Point,
    ) -> Option<Highlight> {
        let row = self.row_of(line)?;
        let start_column = match line == start.line {
            true => start.column.0,
            false => 0,
        };
        let end_column = match line == end.line {
            true => end.column.0 + 1,
            false => self.columns,
        };
        Some(Highlight {
            row,
            start_column,
            end_column,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use alacritty_terminal::index::Column;

    fn viewport() -> Viewport {
        Viewport {
            lines: 4,
            columns: 10,
            display_offset: 2,
        }
    }

    #[test]
    fn rows_and_lines_convert_with_the_scroll_offset() {
        let viewport = viewport();
        assert_eq!(viewport.row_of(Line(-2)), Some(0));
        assert_eq!(viewport.row_of(Line(1)), Some(3));
        assert_eq!(viewport.row_of(Line(2)), None);
        assert_eq!(viewport.line_of(0), Line(-2));
    }

    #[test]
    fn ranges_are_clipped_to_the_visible_rows() {
        let cases = [
            (
                Point::new(Line(-5), Column(3)),
                Point::new(Line(-1), Column(4)),
                vec![(0, 0, 10), (1, 0, 5)],
            ),
            (
                Point::new(Line(0), Column(2)),
                Point::new(Line(0), Column(5)),
                vec![(2, 2, 6)],
            ),
        ];
        for (start, end, expected) in cases {
            let rows: Vec<(usize, usize, usize)> = viewport()
                .highlights(start, end)
                .iter()
                .map(|highlight| (highlight.row, highlight.start_column, highlight.end_column))
                .collect();
            assert_eq!(rows, expected);
        }
    }
}
