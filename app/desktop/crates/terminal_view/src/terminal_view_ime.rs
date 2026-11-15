//! Input-method support: composition shows at the cursor and commits as typed text.

use std::ops::Range;

use gpui::{
    App, Bounds, Context, EntityInputHandler, Pixels, Point, SharedString, TextRun, UTF16Selection,
    UnderlineStyle, Window, fill, point, px, size,
};
use terminal::CursorCell;

use crate::cursor_look::CursorLook;
use crate::paint_cursor::cursor_bounds;
use crate::terminal_view::TerminalView;

const COMPOSITION_UNDERLINE: f32 = 1.0;

impl TerminalView {
    pub(crate) fn paint_preedit(
        &self,
        cell: &CursorCell,
        look: &CursorLook<'_>,
        window: &mut Window,
        cx: &mut App,
    ) {
        let Some(text) = self.preedit.as_ref().filter(|text| !text.is_empty()) else {
            return;
        };
        let metrics = look.layout.metrics;
        let origin = cursor_bounds(cell, &look.layout).origin;
        let cells = text.chars().count();
        let width = px(metrics.cell_width * cells as f32);
        let backdrop = Bounds::new(origin, size(width, px(metrics.line_height)));
        window.paint_quad(fill(backdrop, look.backdrop));
        let line = shape_preedit(text, look, window);
        if let Err(error) = line.paint(origin, px(metrics.line_height), window, cx) {
            tracing::warn!(%error, "terminal composition could not be painted");
        }
    }
}

fn shape_preedit(
    text: &str,
    look: &CursorLook<'_>,
    window: &Window,
) -> gpui::ShapedLine {
    let underline = UnderlineStyle {
        thickness: px(COMPOSITION_UNDERLINE),
        color: Some(look.color),
        wavy: false,
    };
    let run = TextRun {
        len: text.len(),
        font: look.shaping.font.clone(),
        color: look.color,
        background_color: None,
        underline: Some(underline),
        strikethrough: None,
    };
    window.text_system().shape_line(
        SharedString::from(text.to_owned()),
        look.shaping.font_size,
        &[run],
        Some(look.shaping.cell_width),
    )
}

impl EntityInputHandler for TerminalView {
    fn text_for_range(
        &mut self,
        range: Range<usize>,
        adjusted: &mut Option<Range<usize>>,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<String> {
        let text = self.preedit.as_ref()?;
        let units: Vec<u16> = text.encode_utf16().collect();
        let start = range.start.min(units.len());
        let end = range.end.min(units.len());
        *adjusted = Some(start..end);
        String::from_utf16(units.get(start..end)?).ok()
    }

    fn selected_text_range(
        &mut self,
        _: bool,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<UTF16Selection> {
        let end = self
            .preedit
            .as_ref()
            .map_or(0, |text| text.encode_utf16().count());
        Some(UTF16Selection {
            range: end..end,
            reversed: false,
        })
    }

    fn marked_text_range(
        &self,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<Range<usize>> {
        let text = self.preedit.as_ref()?;
        Some(0..text.encode_utf16().count())
    }

    fn unmark_text(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.preedit = None;
        cx.notify();
    }

    fn replace_text_in_range(
        &mut self,
        _: Option<Range<usize>>,
        text: &str,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.preedit = None;
        if !text.is_empty() {
            self.send_text(text, cx);
        }
        cx.notify();
    }

    fn replace_and_mark_text_in_range(
        &mut self,
        _: Option<Range<usize>>,
        text: &str,
        _: Option<Range<usize>>,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.preedit = (!text.is_empty()).then(|| text.to_owned());
        cx.notify();
    }

    fn bounds_for_range(
        &mut self,
        _: Range<usize>,
        _: Bounds<Pixels>,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<Bounds<Pixels>> {
        let (layout, cell) = (self.layout?, self.cursor_cell?);
        let metrics = layout.metrics;
        Some(Bounds::new(
            point(
                layout.origin.x + px(metrics.cell_width * cell.column as f32),
                layout.origin.y + px(metrics.line_height * cell.row as f32),
            ),
            size(px(metrics.cell_width), px(metrics.line_height)),
        ))
    }

    fn character_index_for_point(
        &mut self,
        _: Point<Pixels>,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<usize> {
        None
    }
}

#[cfg(test)]
mod tests {
    use gpui::{EntityInputHandler, TestAppContext};

    use crate::test_support::{open, typed};

    #[gpui::test]
    fn composition_is_held_until_committed_as_typed_text(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx);
        harness.view.update_in(cx, |view, window, cx| {
            view.replace_and_mark_text_in_range(None, "ni", None, window, cx);
        });
        assert_eq!(typed(&harness), "");
        let marked = harness
            .view
            .update_in(cx, |view, window, cx| view.marked_text_range(window, cx));
        assert_eq!(marked, Some(0..2));
        harness.view.update_in(cx, |view, window, cx| {
            view.replace_text_in_range(None, "你", window, cx);
        });
        assert_eq!(typed(&harness), "你");
        let marked = harness
            .view
            .update_in(cx, |view, window, cx| view.marked_text_range(window, cx));
        assert_eq!(marked, None);
    }
}
