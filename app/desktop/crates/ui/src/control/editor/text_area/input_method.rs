//! Input-method and text-service interface of [`TextArea`].

use std::ops::Range;

use gpui::{
    Bounds, Context, EntityInputHandler, Pixels, Point, UTF16Selection, Window, point, size,
};

use crate::control::editor::text_area::TextArea;
use crate::control::editor::text_area::editing::edit_kind;
use crate::control::editor::text_history::Edit;
use crate::control::editor::text_input::CARET_WIDTH;

type IgnoreDisabledInput = bool;

impl EntityInputHandler for TextArea {
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
        _: IgnoreDisabledInput,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<UTF16Selection> {
        Some(UTF16Selection {
            range: self.buffer.range_to_utf16(&self.buffer.selection()),
            reversed: self.buffer.is_reversed(),
        })
    }

    fn marked_text_range(
        &self,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<Range<usize>> {
        self.buffer
            .marked()
            .map(|range| self.buffer.range_to_utf16(&range))
    }

    fn unmark_text(
        &mut self,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) {
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
        let kind = edit_kind(text, &self.buffer.selection());
        self.edit(kind, cx, |buffer| buffer.replace(range, text));
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
        self.edit(Edit::Other, cx, |buffer| {
            buffer.replace_and_mark(range, text, selected);
        });
    }

    fn bounds_for_range(
        &mut self,
        range: Range<usize>,
        bounds: Bounds<Pixels>,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<Bounds<Pixels>> {
        let range = self.buffer.range_from_utf16(&range);
        let start = self.layout.point_for_index(range.start);
        let end = self.layout.point_for_index(range.end);
        let origin = point(bounds.left(), bounds.top() - self.scroll);
        let width = (end.x - start.x).max(gpui::px(CARET_WIDTH));
        Some(Bounds::new(
            origin + start,
            size(width, self.layout.line_height()),
        ))
    }

    fn character_index_for_point(
        &mut self,
        position: Point<Pixels>,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> Option<usize> {
        let bounds = self.bounds?;
        let local = bounds.localize(&position)?;
        let local = point(local.x, local.y + self.scroll);
        let index = self.layout.index_for_point(local);
        Some(self.buffer.utf16_offset(index))
    }
}

#[cfg(test)]
mod tests {
    use gpui::{EntityInputHandler as _, TestAppContext};

    use crate::control::editor::test_support::area::{area, area_text};

    #[gpui::test]
    fn composition_is_marked_then_replaced_and_undoes_as_one_step(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        let marked = area.update_in(cx, |area, window, cx| {
            area.replace_and_mark_text_in_range(None, "ni", None, window, cx);
            area.marked_text_range(window, cx)
        });
        assert_eq!(marked, Some(0..2));
        area.update_in(cx, |area, window, cx| {
            area.replace_and_mark_text_in_range(None, "\u{4f60}", None, window, cx);
            area.replace_text_in_range(None, "\u{4f60}", window, cx);
        });
        assert_eq!(area_text(&area, cx), "\u{4f60}");
        cx.simulate_keystrokes("secondary-z");
        assert_eq!(area_text(&area, cx), "");
    }

    #[gpui::test]
    fn ranges_are_reported_in_utf16_units_across_lines(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        cx.simulate_input("a😀");
        cx.simulate_keystrokes("enter");
        cx.simulate_input("b");
        let selection = area.update_in(cx, |area, window, cx| {
            area.selected_text_range(false, window, cx)
        });
        assert_eq!(selection.map(|selection| selection.range), Some(5..5));
        let slice = area.update_in(cx, |area, window, cx| {
            area.text_for_range(1..3, &mut None, window, cx)
        });
        assert_eq!(slice.as_deref(), Some("😀"));
    }

    #[gpui::test]
    fn the_caret_rectangle_of_a_later_line_sits_below_the_first(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        cx.simulate_input("a");
        cx.simulate_keystrokes("enter");
        cx.simulate_input("b");
        let heights = area.update_in(cx, |area, window, cx| {
            let bounds = area.bounds.unwrap_or_default();
            let first = area.bounds_for_range(0..0, bounds, window, cx);
            let last = area.bounds_for_range(2..2, bounds, window, cx);
            first.zip(last).map(|(a, b)| f32::from(b.top() - a.top()))
        });
        assert_eq!(heights, Some(20.0));
    }
}
