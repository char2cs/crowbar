use gpui::{Hsla, Window, fill, point, px, size};
use terminal::Highlight;

use crate::terminal_view::paint::view_layout::ViewLayout;

const UNDERLINE_THICKNESS: f32 = 1.0;

pub(crate) fn paint_fills(
    highlights: &[Highlight],
    layout: &ViewLayout,
    color: Hsla,
    window: &mut Window,
) {
    for highlight in highlights {
        window.paint_quad(fill(cell_rectangle(highlight, layout), color));
    }
}

pub(crate) fn paint_underlines(
    highlights: &[Highlight],
    layout: &ViewLayout,
    color: Hsla,
    window: &mut Window,
) {
    for highlight in highlights {
        let cells = cell_rectangle(highlight, layout);
        let thickness = px(UNDERLINE_THICKNESS);
        let line = gpui::Bounds::new(
            point(cells.origin.x, cells.bottom() - thickness),
            size(cells.size.width, thickness),
        );
        window.paint_quad(fill(line, color));
    }
}

fn cell_rectangle(
    highlight: &Highlight,
    layout: &ViewLayout,
) -> gpui::Bounds<gpui::Pixels> {
    let metrics = layout.metrics;
    let x = layout.origin.x + px(metrics.cell_width * terminal::cells_f32(highlight.start_column));
    let y = layout.origin.y + px(metrics.line_height * terminal::cells_f32(highlight.row));
    let columns = highlight.end_column.saturating_sub(highlight.start_column);
    gpui::Bounds::new(
        point(x, y),
        size(
            px(metrics.cell_width * terminal::cells_f32(columns)),
            px(metrics.line_height),
        ),
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use gpui::Bounds;
    use terminal::CellMetrics;

    #[test]
    fn a_highlight_covers_its_cells() {
        let bounds = Bounds::new(point(px(10.0), px(20.0)), size(px(300.0), px(200.0)));
        let layout = ViewLayout::new(bounds, (8.0, 16.0), 0.0);
        assert_eq!(
            layout.metrics,
            CellMetrics {
                cell_width: 8.0,
                line_height: 16.0,
                columns: 35,
                lines: 12
            }
        );
        let rectangle = cell_rectangle(
            &Highlight {
                row: 2,
                start_column: 3,
                end_column: 6,
            },
            &layout,
        );
        assert_eq!(
            rectangle,
            Bounds::new(point(px(34.0), px(52.0)), size(px(24.0), px(16.0)))
        );
    }
}
