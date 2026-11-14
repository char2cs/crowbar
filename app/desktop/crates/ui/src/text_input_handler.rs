//! Input-method and text-service interface of [`TextInput`].

use std::ops::Range;

use gpui::{Bounds, Context, EntityInputHandler, Pixels, Point, UTF16Selection, Window};

use crate::text_input::TextInput;

impl EntityInputHandler for TextInput {
    fn text_for_range(
        &mut self,
        range_utf16: Range<usize>,
        actual_range: &mut Option<Range<usize>>,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<String> {
        let range = self.buffer.range_from_utf16(&range_utf16);
        actual_range.replace(self.buffer.range_to_utf16(&range));
        self.buffer.text().get(range).map(str::to_owned)
    }

    fn selected_text_range(
        &mut self,
        _: bool,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<UTF16Selection> {
        Some(UTF16Selection {
            range: self.buffer.range_to_utf16(&self.buffer.selection()),
            reversed: self.buffer.is_reversed(),
        })
    }

    fn marked_text_range(&self, _: &mut Window, _: &mut Context<'_, Self>) -> Option<Range<usize>> {
        self.buffer
            .marked()
            .map(|range| self.buffer.range_to_utf16(&range))
    }

    fn unmark_text(&mut self, _: &mut Window, _: &mut Context<'_, Self>) {
        self.buffer.unmark();
    }

    fn replace_text_in_range(
        &mut self,
        range: Option<Range<usize>>,
        text: &str,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let range = range.map(|range| self.buffer.range_from_utf16(&range));
        self.buffer.replace(range, text);
        self.changed(cx);
    }

    fn replace_and_mark_text_in_range(
        &mut self,
        range: Option<Range<usize>>,
        text: &str,
        selected: Option<Range<usize>>,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let range = range.map(|range| self.buffer.range_from_utf16(&range));
        let selected = selected.map(|range| self.buffer.range_from_utf16(&range));
        self.buffer.replace_and_mark(range, text, selected);
        self.changed(cx);
    }

    fn bounds_for_range(
        &mut self,
        range: Range<usize>,
        bounds: Bounds<Pixels>,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<Bounds<Pixels>> {
        let line = self.layout.as_ref()?;
        let range = self.buffer.range_from_utf16(&range);
        Some(Bounds::from_corners(
            gpui::point(bounds.left() + line.x_for_index(range.start), bounds.top()),
            gpui::point(bounds.left() + line.x_for_index(range.end), bounds.bottom()),
        ))
    }

    fn character_index_for_point(
        &mut self,
        point: Point<Pixels>,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<usize> {
        let local = self.bounds?.localize(&point)?;
        let line = self.layout.as_ref()?;
        let index = line.index_for_x(local.x)?;
        Some(self.buffer.utf16_offset(index))
    }
}

#[cfg(test)]
#[path = "text_input_handler_tests.rs"]
mod tests;
