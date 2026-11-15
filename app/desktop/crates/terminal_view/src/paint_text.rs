//! Paints cell backgrounds and text.

use gpui::{App, Bounds, Pixels, ShapedLine, Window, fill, point, px, size};
use terminal::{BackgroundSpan, Row};

use crate::frame::Frame;
use crate::line_cache::LineCache;
use crate::theme_palette::hsla_of;
use crate::view_layout::ViewLayout;

pub(crate) fn paint_backgrounds(
    frame: &Frame,
    window: &mut Window,
) {
    for (index, row) in frame.snapshot.rows.iter().enumerate() {
        paint_row_backgrounds(&frame.layout, row, index, window);
    }
}

fn paint_row_backgrounds(
    layout: &ViewLayout,
    row: &Row,
    index: usize,
    window: &mut Window,
) {
    let top = row_top(layout, index);
    for span in &row.backgrounds {
        paint_background(layout, span, top, window);
    }
}

fn row_top(
    layout: &ViewLayout,
    index: usize,
) -> Pixels {
    layout.origin.y + px(layout.metrics.line_height * index as f32)
}

fn paint_background(
    layout: &ViewLayout,
    span: &BackgroundSpan,
    top: Pixels,
    window: &mut Window,
) {
    let metrics = layout.metrics;
    let left = layout.origin.x + px(metrics.cell_width * span.column as f32);
    let extent = size(
        px(metrics.cell_width * span.columns as f32),
        px(metrics.line_height),
    );
    window.paint_quad(fill(
        Bounds::new(point(left, top), extent),
        hsla_of(span.color),
    ));
}

pub(crate) fn paint_rows(
    frame: &Frame,
    cache: &mut LineCache,
    window: &mut Window,
    cx: &mut App,
) {
    for (index, row) in frame.snapshot.rows.iter().enumerate() {
        let lines = cache.lines(index, row, &frame.shaping, window).to_vec();
        paint_row_lines(&frame.layout, (index, row, &lines), window, cx);
    }
}

fn paint_row_lines(
    layout: &ViewLayout,
    (index, row, lines): (usize, &Row, &[ShapedLine]),
    window: &mut Window,
    cx: &mut App,
) {
    let top = row_top(layout, index);
    let height = px(layout.metrics.line_height);
    for (column, line) in segment_columns(row).zip(lines) {
        let left = layout.origin.x + px(layout.metrics.cell_width * column as f32);
        let bounds = Bounds::new(point(left, top), size(px(0.0), height));
        paint_line(line, bounds, window, cx);
    }
}

fn paint_line(
    line: &ShapedLine,
    bounds: Bounds<Pixels>,
    window: &mut Window,
    cx: &mut App,
) {
    if let Err(error) = line.paint(bounds.origin, bounds.size.height, window, cx) {
        tracing::warn!(%error, "terminal row could not be painted");
    }
}

fn segment_columns(row: &Row) -> impl Iterator<Item = usize> + '_ {
    row.segments.iter().map(|segment| segment.column)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rows_stack_by_line_height() {
        let bounds = Bounds::new(point(px(0.0), px(10.0)), size(px(400.0), px(200.0)));
        let layout = ViewLayout::new(bounds, (8.0, 16.0), 0.0);
        assert_eq!(row_top(&layout, 0), px(10.0));
        assert_eq!(row_top(&layout, 3), px(58.0));
    }
}
