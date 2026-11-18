//! Paints the cursor.

use gpui::{
    App, BorderStyle, Bounds, Pixels, SharedString, TextRun, Window, fill, outline, point, px, size,
};
use terminal::{CursorCell, CursorShape};

use crate::cursor_look::CursorLook;
use crate::focus_state::FocusState;
use crate::view_layout::ViewLayout;

pub(crate) fn cursor_bounds(
    cell: &CursorCell,
    layout: &ViewLayout,
) -> Bounds<Pixels> {
    let metrics = layout.metrics;
    Bounds::new(
        point(
            layout.origin.x + px(metrics.cell_width * terminal::cells_f32(cell.column)),
            layout.origin.y + px(metrics.line_height * terminal::cells_f32(cell.row)),
        ),
        size(
            px(metrics.cell_width * terminal::cells_f32(cell.columns)),
            px(metrics.line_height),
        ),
    )
}

pub(crate) fn paint_cursor(
    cell: &CursorCell,
    look: &CursorLook<'_>,
    window: &mut Window,
    cx: &mut App,
) {
    let cells = cursor_bounds(cell, &look.layout);
    match (cell.shape, look.focus) {
        (CursorShape::Block, FocusState::Focused) => paint_block(cell, look, window, cx),
        (CursorShape::Block, FocusState::Unfocused) => {
            window.paint_quad(outline(cells, look.color, BorderStyle::Solid));
        },
        (CursorShape::Bar, _) => {
            let bar = Bounds::new(cells.origin, size(px(look.thickness), cells.size.height));
            window.paint_quad(fill(bar, look.color));
        },
        (CursorShape::Underline, _) => {
            let line = Bounds::new(
                point(cells.origin.x, cells.bottom() - px(look.thickness)),
                size(cells.size.width, px(look.thickness)),
            );
            window.paint_quad(fill(line, look.color));
        },
    }
}

fn paint_block(
    cell: &CursorCell,
    look: &CursorLook<'_>,
    window: &mut Window,
    cx: &mut App,
) {
    let cells = cursor_bounds(cell, &look.layout);
    window.paint_quad(fill(cells, look.color));
    if cell.character == ' ' {
        return;
    }
    let text = SharedString::from(cell.character.to_string());
    let run = TextRun {
        len: text.len(),
        font: look.shaping.font.clone(),
        color: look.glyph_color,
        background_color: None,
        underline: None,
        strikethrough: None,
    };
    let width = look.shaping.cell_width * terminal::cells_f32(cell.columns);
    let line = window
        .text_system()
        .shape_line(text, look.shaping.font_size, &[run], Some(width));
    if let Err(error) = line.paint(cells.origin, cells.size.height, window, cx) {
        tracing::warn!(%error, "terminal cursor glyph could not be painted");
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use terminal::CellMetrics;

    #[test]
    fn the_cursor_box_sits_on_its_cells() {
        let metrics = CellMetrics {
            cell_width: 8.0,
            line_height: 16.0,
            columns: 80,
            lines: 24,
        };
        let layout = ViewLayout::new(
            Bounds::new(point(px(0.0), px(0.0)), size(px(800.0), px(400.0))),
            (metrics.cell_width, metrics.line_height),
            0.0,
        );
        let cell = CursorCell {
            row: 2,
            column: 3,
            columns: 2,
            shape: CursorShape::Block,
            character: 'x',
        };
        let bounds = cursor_bounds(&cell, &layout);
        assert_eq!(bounds.origin, point(px(24.0), px(32.0)));
        assert_eq!(bounds.size, size(px(16.0), px(16.0)));
    }
}
