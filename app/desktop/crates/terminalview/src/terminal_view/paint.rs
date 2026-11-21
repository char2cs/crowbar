//! Layout and painting of the grid.

mod cell_size;
pub(crate) mod colors;
pub(crate) mod cursor;
mod frame;
mod highlights;
mod layers;
pub(crate) mod request;
mod scrollbar;
pub(crate) mod shaping;
mod text;
pub(crate) mod view_layout;

pub(crate) use cursor::{BLINK_INTERVAL, FocusState, Look, PreeditSite, cursor_bounds};

use gpui::{App, Bounds, Context, ElementInputHandler, Pixels, Window, px};
use terminal::{ScrollbarThumb, Snapshot};

use crate::settings::base_font;
use crate::settings::terminal_palette;
use crate::terminal_view::TerminalView;
use crate::terminal_view::input::pointer::drag_state::DragState;
use crate::terminal_view::paint::cell_size::cell_size;
use crate::terminal_view::paint::colors::Colors;
use crate::terminal_view::paint::cursor::paint_cursor;
use crate::terminal_view::paint::frame::Frame;
use crate::terminal_view::paint::highlights::{paint_fills, paint_underlines};
use crate::terminal_view::paint::layers::Layers;
use crate::terminal_view::paint::request::Request;
use crate::terminal_view::paint::scrollbar::paint_scrollbar;
use crate::terminal_view::paint::shaping::Shaping;
use crate::terminal_view::paint::text::{paint_backgrounds, paint_rows};
use crate::terminal_view::paint::view_layout::ViewLayout;

const MEASURE_GLYPH: char = 'm';

impl TerminalView {
    fn measure_cell(
        &self,
        window: &Window,
    ) -> (f32, f32) {
        let text_system = window.text_system();
        let font_id = text_system.resolve_font(&base_font(&self.settings));
        let size = px(self.settings.font_size);
        let advance = text_system
            .advance(font_id, size, MEASURE_GLYPH)
            .map_or(self.settings.font_size, |advance| f32::from(advance.width));
        let ascent = f32::from(text_system.ascent(font_id, size));
        let descent = -f32::from(text_system.descent(font_id, size));
        cell_size((advance, ascent, descent), &self.settings)
    }

    fn shaping(
        &self,
        cell_width: f32,
    ) -> Shaping {
        Shaping {
            font: base_font(&self.settings),
            font_size: px(self.settings.font_size),
            cell_width: px(cell_width),
        }
    }

    pub(crate) fn lay_out(
        &mut self,
        bounds: Bounds<Pixels>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Option<Frame> {
        if bounds.size.width <= px(0.0) || bounds.size.height <= px(0.0) {
            return None;
        }
        let cell = self.measure_cell(window);
        let layout = ViewLayout::new(bounds, cell, self.inset);
        if self.cell_changed(cell) {
            self.cache.clear();
        }
        self.layout = Some(layout);
        self.fit_grid(&layout, cx);
        let snapshot = self.terminal.snapshot();
        self.cursor_cell = snapshot.cursor;
        let shaping = self.shaping(cell.0);
        Some(Frame {
            snapshot,
            layout,
            shaping,
        })
    }

    fn cell_changed(
        &self,
        cell: (f32, f32),
    ) -> bool {
        self.layout.is_none_or(|previous| {
            !same_size(previous.metrics.cell_width, cell.0)
                || !same_size(previous.metrics.line_height, cell.1)
        })
    }

    fn fit_grid(
        &mut self,
        layout: &ViewLayout,
        cx: &mut Context<'_, Self>,
    ) {
        let size = (layout.metrics.columns, layout.metrics.lines);
        if self.reported_size == Some(size) {
            return;
        }
        self.reported_size = Some(size);
        self.terminal.resize(size.0, size.1);
        self.cache.clear();
        if let Some(frames) = self.unread.take() {
            self.start_frame_task(frames, cx);
        }
        let Ok(columns) = u16::try_from(size.0) else {
            return;
        };
        let Ok(rows) = u16::try_from(size.1) else {
            return;
        };
        self.send(terminal::ClientMessage::Resize { columns, rows });
    }

    pub(crate) fn paint_frame(
        &mut self,
        request: Request,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Request { bounds, frame } = request;
        let Some(frame) = frame else {
            return;
        };
        let colors = Colors::of(cx);
        paint_backgrounds(&frame, window);
        Self::paint_highlights(&frame, &colors, window);
        paint_rows(&frame, &mut self.cache, window, cx);
        self.paint_links(&frame, &colors, window);
        let layers = Layers {
            frame: &frame,
            colors: &colors,
        };
        self.paint_cursor_cell(layers, window, cx);
        self.paint_scrollbar_thumb(&frame, &colors, window);
        Self::follow_pointer_while_painted(bounds, window, cx);
        let handler = ElementInputHandler::new(bounds, cx.entity());
        window.handle_input(&self.focus, handler, cx);
    }

    fn paint_highlights(
        frame: &Frame,
        colors: &Colors,
        window: &mut Window,
    ) {
        let layers = [
            (&frame.snapshot.matches, colors.matched),
            (&frame.snapshot.current_match, colors.current_match),
            (&frame.snapshot.selection, colors.selection),
        ];
        for (cells, color) in layers {
            paint_fills(cells, &frame.layout, color, window);
        }
    }

    fn paint_links(
        &self,
        frame: &Frame,
        colors: &Colors,
        window: &mut Window,
    ) {
        paint_underlines(
            self.hovered_cells(),
            &frame.layout,
            colors.foreground,
            window,
        );
    }

    fn paint_cursor_cell(
        &self,
        layers: Layers<'_>,
        window: &mut Window,
        cx: &mut App,
    ) {
        let Layers { frame, colors } = layers;
        let Some(cell) = frame.snapshot.cursor else {
            return;
        };
        if !self.cursor_visible(&frame.snapshot) {
            return;
        }
        let palette = terminal_palette(cx);
        let focus = FocusState::from(self.focused);
        let look = Look {
            color: colors.foreground,
            glyph_color: crate::settings::hsla_of(palette.background),
            backdrop: colors.pane,
            thickness: self.settings.cursor_width,
            focus,
            shaping: &frame.shaping,
            layout: frame.layout,
        };
        paint_cursor(&cell, &look, window, cx);
        let site = PreeditSite {
            cell: &cell,
            look: &look,
        };
        self.paint_preedit(site, window, cx);
    }

    fn cursor_visible(
        &self,
        snapshot: &Snapshot,
    ) -> bool {
        !self.focused || !snapshot.cursor_blinks || self.blink.is_shown() || self.preedit.is_some()
    }

    fn paint_scrollbar_thumb(
        &self,
        frame: &Frame,
        colors: &Colors,
        window: &mut Window,
    ) {
        let dragging = matches!(self.drag, DragState::ScrollbarThumb { .. });
        if self.pointer.is_none() && !dragging {
            return;
        }
        let over_track = self
            .pointer
            .is_some_and(|pointer| frame.layout.track.contains(&pointer));
        let color = match (dragging, over_track) {
            (false, false) => colors.thumb,
            _ => colors.thumb_hover,
        };
        let snapshot = &frame.snapshot;
        let track = f32::from(frame.layout.track.size.height);
        let thumb = ScrollbarThumb::new(
            track,
            frame.layout.metrics.lines,
            snapshot.history_size,
            snapshot.display_offset,
        );
        if let Some(thumb) = thumb {
            paint_scrollbar(thumb, &frame.layout, color, window);
        }
    }
}

fn same_size(
    left: f32,
    right: f32,
) -> bool {
    (left - right).abs() < f32::EPSILON
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sizes_within_epsilon_are_the_same() {
        assert!(same_size(8.0, 8.0), "equal sizes");
        assert!(!same_size(8.0, 9.0), "different sizes");
    }
}
