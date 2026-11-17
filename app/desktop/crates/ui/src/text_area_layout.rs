//! Wrapped lines of a text area with the geometry the caret, selection and mouse need.

use gpui::{Pixels, Point, Window, WrappedLine, point, px};

use crate::direction::Direction;
use crate::pose::Pose;
use crate::shaping::Shaping;
use crate::text_area_rows::{edges_of, span_of, text_row};
use crate::text_row::TextRow;

#[derive(Debug, Clone, Default)]
pub(crate) struct TextAreaLayout {
    pub(crate) lines: Vec<WrappedLine>,
    pub(crate) starts: Vec<usize>,
    pub(crate) rows: Vec<TextRow>,
    pub(crate) line_height: Pixels,
}

impl TextAreaLayout {
    pub(crate) fn shape(
        shaping: Shaping<'_>,
        window: &Window,
    ) -> Self {
        let lines = window.text_system().shape_text(
            shaping.text,
            shaping.font_size,
            shaping.runs,
            shaping.wrap_width,
            None,
        );
        let lines = lines.map_or_default(|lines| lines.into_iter().collect());
        Self::from_lines(lines, shaping.line_height)
    }

    pub(crate) fn from_lines(
        lines: Vec<WrappedLine>,
        line_height: Pixels,
    ) -> Self {
        let mut layout = Self {
            starts: Vec::with_capacity(lines.len()),
            rows: Vec::new(),
            lines,
            line_height,
        };
        let mut start = 0;
        for line in 0..layout.lines.len() {
            layout.starts.push(start);
            start += layout.add_rows(line, start) + 1;
        }
        layout
    }

    fn add_rows(
        &mut self,
        line: usize,
        start: usize,
    ) -> usize {
        let Some(wrapped) = self.lines.get(line) else {
            return 0;
        };
        let edges = edges_of(wrapped);
        let last = edges.len().saturating_sub(2);
        for (position, pair) in edges.windows(2).enumerate() {
            let closes = Pose::from(position == last);
            let top = self.line_height * self.rows.len();
            let row = text_row(wrapped, start, span_of(pair), closes);
            self.rows.push(TextRow { top, line, ..row });
        }
        wrapped.len()
    }

    pub(crate) fn height(&self) -> Pixels {
        self.line_height * self.rows.len().max(1)
    }

    pub(crate) fn width(&self) -> Pixels {
        self.lines
            .iter()
            .map(|line| line.width())
            .fold(px(0.0), Pixels::max)
    }

    pub(crate) fn line_height(&self) -> Pixels {
        self.line_height
    }

    pub(crate) fn row_position(
        &self,
        index: usize,
    ) -> Option<usize> {
        self.rows.iter().position(|row| row.holds(index))
    }

    pub(crate) fn x_in(
        &self,
        row: &TextRow,
        index: usize,
    ) -> Pixels {
        let start = self.starts.get(row.line).copied().unwrap_or(0);
        let x = self.lines.get(row.line).map_or(px(0.0), |line| {
            line.unwrapped_layout
                .x_for_index(index.saturating_sub(start))
        });
        x - row.origin
    }

    pub(crate) fn point_for_index(
        &self,
        index: usize,
    ) -> Point<Pixels> {
        let row = self.row_position(index).and_then(|at| self.rows.get(at));
        row.map_or_else(Point::default, |row| point(self.x_in(row, index), row.top))
    }

    fn index_in_row(
        &self,
        row: &TextRow,
        x: Pixels,
    ) -> usize {
        let Some(line) = self.lines.get(row.line) else {
            return 0;
        };
        let start = self.starts.get(row.line).copied().unwrap_or(0);
        let local = line.unwrapped_layout.closest_index_for_x(x + row.origin);
        (start + local).clamp(row.bytes.start, row.caret_end)
    }

    pub(crate) fn index_for_point(
        &self,
        position: Point<Pixels>,
    ) -> usize {
        let hit = self.rows.iter().rposition(|row| row.top <= position.y);
        let row = hit.and_then(|at| self.rows.get(at)).or(self.rows.first());
        row.map_or(0, |row| self.index_in_row(row, position.x))
    }

    pub(crate) fn step_vertically(
        &self,
        index: usize,
        goal: Pixels,
        direction: Direction,
    ) -> usize {
        let Some(at) = self.row_position(index) else {
            return index;
        };
        let target = match direction {
            Direction::Backward => at.checked_sub(1),
            Direction::Forward => Some(at + 1),
        };
        if let Some(row) = target.and_then(|target| self.rows.get(target)) {
            return self.index_in_row(row, goal);
        }
        match direction {
            Direction::Backward => 0,
            Direction::Forward => self.rows.last().map_or(0, |row| row.caret_end),
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, px};

    use super::*;
    use crate::test_support::text_layouts::{LINE, layout_of, window};

    #[gpui::test]
    fn each_newline_starts_a_row_and_empty_lines_keep_one(cx: &mut TestAppContext) {
        const CASES: &[(&str, usize)] =
            &[("", 1), ("a", 1), ("a\nb", 2), ("a\n\nb", 3), ("a\n", 2)];
        let cx = window(cx);
        for &(text, rows) in CASES {
            let layout = layout_of(text, None, cx);
            assert_eq!(layout.rows.len(), rows, "{text:?}");
            assert_eq!(layout.height(), px(LINE) * rows, "{text:?}");
        }
    }

    #[gpui::test]
    fn a_narrow_width_wraps_a_long_line_into_more_rows(cx: &mut TestAppContext) {
        let cx = window(cx);
        let text = "alpha beta gamma delta epsilon zeta";
        let wide = layout_of(text, Some(600.0), cx);
        let narrow = layout_of(text, Some(60.0), cx);
        assert_eq!(wide.rows.len(), 1);
        assert!(narrow.rows.len() > 2, "{} rows", narrow.rows.len());
        let tops: Vec<Pixels> = narrow.rows.iter().map(|row| row.top).collect();
        assert!(tops.windows(2).all(|pair| pair.first() < pair.last()));
        assert_eq!(
            narrow.rows.last().map(|row| row.bytes.end),
            Some(text.len())
        );
    }

    #[gpui::test]
    fn indices_map_to_points_and_back(cx: &mut TestAppContext) {
        const CASES: &[(usize, f32)] = &[(0, 0.0), (3, 0.0), (4, LINE), (6, LINE)];
        let cx = window(cx);
        let layout = layout_of("abc\nde", None, cx);
        for &(index, top) in CASES {
            assert_eq!(layout.point_for_index(index).y, px(top), "{index}");
        }
        for index in [0, 2, 3, 4, 5, 6] {
            let spot = layout.point_for_index(index);
            let back = layout.index_for_point(point(spot.x, spot.y + px(1.0)));
            assert_eq!(back, index, "{index}");
        }
    }

    #[gpui::test]
    fn points_outside_the_text_clamp_to_its_ends(cx: &mut TestAppContext) {
        const CASES: &[(f32, f32, usize)] =
            &[(-50.0, -50.0, 0), (900.0, 5.0, 3), (900.0, 500.0, 6)];
        let cx = window(cx);
        let layout = layout_of("abc\nde", None, cx);
        for &(x, y, index) in CASES {
            assert_eq!(
                layout.index_for_point(point(px(x), px(y))),
                index,
                "{x},{y}"
            );
        }
    }

    #[gpui::test]
    fn vertical_steps_stop_at_the_document_edges(cx: &mut TestAppContext) {
        let cx = window(cx);
        let layout = layout_of("abc\nde", None, cx);
        let up = layout.step_vertically(1, px(0.0), Direction::Backward);
        let down = layout.step_vertically(5, px(0.0), Direction::Forward);
        assert_eq!((up, down), (0, 6));
        assert_eq!(layout.step_vertically(1, px(0.0), Direction::Forward), 4);
    }
}
