use std::ops::Range;

use gpui::{App, Bounds, Pixels, Point, TextAlign, Window, point, px, size};

use crate::control::editor::text_area::text_area_layout::TextAreaLayout;
use crate::control::editor::text_area::text_area_layout::text_row::TextRow;

const NEWLINE_WIDTH: f32 = 4.0;

impl TextAreaLayout {
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
    use gpui::{TestAppContext, px};

    use crate::control::editor::text_area::text_area_layout::test_support::{
        LINE, layout_of, window,
    };

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
}
