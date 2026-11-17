//! Wrapped lines of a text area with the geometry the caret, selection and mouse need.

use std::ops::Range;

use gpui::{App, Bounds, Pixels, Point, TextAlign, Window, WrappedLine, point, px, size};

use crate::direction::Direction;
use crate::pose::Pose;
use crate::shaping::Shaping;
use crate::text_row::TextRow;

const NEWLINE_WIDTH: f32 = 4.0;

#[derive(Debug, Clone, Default)]
pub(crate) struct TextAreaLayout {
    lines: Vec<WrappedLine>,
    starts: Vec<usize>,
    rows: Vec<TextRow>,
    line_height: Pixels,
}

fn break_index(
    wrapped: &WrappedLine,
    boundary: &gpui::WrapBoundary,
) -> usize {
    wrapped
        .runs()
        .get(boundary.run_ix)
        .and_then(|run| run.glyphs.get(boundary.glyph_ix))
        .map_or(0, |glyph| glyph.index)
}

fn caret_before(
    wrapped: &WrappedLine,
    end: usize,
) -> usize {
    wrapped
        .text
        .get(..end)
        .and_then(|head| head.chars().next_back())
        .map_or(end, |last| end - last.len_utf8())
}

fn caret_edge(
    wrapped: &WrappedLine,
    to: usize,
    closes_line: Pose,
) -> usize {
    if closes_line == Pose::On {
        return to;
    }
    caret_before(wrapped, to)
}

fn text_row(
    wrapped: &WrappedLine,
    start: usize,
    span: Range<usize>,
    closes: Pose,
) -> TextRow {
    TextRow {
        bytes: start + span.start..start + span.end,
        caret_end: start + caret_edge(wrapped, span.end, closes),
        top: px(0.0),
        origin: wrapped.unwrapped_layout.x_for_index(span.start),
        line: 0,
        closes_line: closes == Pose::On,
    }
}

fn span_of(pair: &[usize]) -> Range<usize> {
    pair.first().copied().unwrap_or(0)..pair.last().copied().unwrap_or(0)
}

fn edges_of(wrapped: &WrappedLine) -> Vec<usize> {
    let breaks = wrapped
        .wrap_boundaries()
        .iter()
        .map(|boundary| break_index(wrapped, boundary));
    std::iter::once(0)
        .chain(breaks)
        .chain(std::iter::once(wrapped.len()))
        .collect()
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

    fn row_position(
        &self,
        index: usize,
    ) -> Option<usize> {
        self.rows.iter().position(|row| row.holds(index))
    }

    fn x_in(
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

    pub(crate) fn row_bounds(
        &self,
        index: usize,
    ) -> Range<usize> {
        let row = self.row_position(index).and_then(|at| self.rows.get(at));
        row.map_or(index..index, |row| row.bytes.start..row.caret_end)
    }

    fn band(
        &self,
        row: &TextRow,
        selection: &Range<usize>,
    ) -> Option<Bounds<Pixels>> {
        let touches = selection.start <= row.bytes.end && selection.end > row.bytes.start;
        let from = selection.start.max(row.bytes.start);
        let to = selection.end.min(row.bytes.end);
        let newline = row.closes_line && selection.end > row.bytes.end;
        let blank = from >= to && !newline;
        if !touches || blank {
            return None;
        }
        let extra = px(NEWLINE_WIDTH * f32::from(u8::from(newline)));
        let (left, right) = (self.x_in(row, from), self.x_in(row, to) + extra);
        Some(Bounds::new(
            point(left, row.top),
            size(right - left, self.line_height),
        ))
    }

    pub(crate) fn selection_bounds(
        &self,
        selection: &Range<usize>,
    ) -> Vec<Bounds<Pixels>> {
        self.rows
            .iter()
            .filter_map(|row| self.band(row, selection))
            .collect()
    }

    pub(crate) fn paint(
        &self,
        origin: Point<Pixels>,
        window: &mut Window,
        cx: &mut App,
    ) {
        for (line, wrapped) in self.lines.iter().enumerate() {
            let top = self
                .rows
                .iter()
                .find(|row| row.line == line)
                .map_or(px(0.0), |row| row.top);
            let painted = wrapped.paint(
                point(origin.x, origin.y + top),
                self.line_height,
                TextAlign::Left,
                None,
                window,
                cx,
            );
            debug_assert!(painted.is_ok(), "wrapped line failed to paint");
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, TextRun, VisualTestContext, px};

    use super::*;

    const LINE: f32 = 20.0;

    fn run_of(text: &str) -> TextRun {
        TextRun {
            len: text.len(),
            font: gpui::font("Cal Sans UI"),
            color: gpui::black(),
            background_color: None,
            underline: None,
            strikethrough: None,
        }
    }

    fn layout_of(
        text: &'static str,
        wrap: Option<f32>,
        cx: &mut VisualTestContext,
    ) -> TextAreaLayout {
        let runs = [run_of(text)];
        let shaping = Shaping {
            text: text.into(),
            font_size: px(14.0),
            runs: &runs,
            wrap_width: wrap.map(px),
            line_height: px(LINE),
        };
        cx.update(|window, _| TextAreaLayout::shape(shaping, window))
    }

    fn window(cx: &mut TestAppContext) -> &mut VisualTestContext {
        crate::test_support::area(cx).1
    }

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
    fn selections_become_one_band_per_row(cx: &mut TestAppContext) {
        let cx = window(cx);
        let layout = layout_of("abc\n\nde", None, cx);
        let bands = layout.selection_bounds(&(1..6));
        assert_eq!(bands.len(), 3);
        let tops: Vec<f32> = bands.iter().map(|band| f32::from(band.top())).collect();
        assert_eq!(tops, [0.0, LINE, 2.0 * LINE]);
        assert!(bands.iter().all(|band| band.size.width > px(0.0)));
        assert_eq!(layout.selection_bounds(&(2..2)), Vec::new());
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
