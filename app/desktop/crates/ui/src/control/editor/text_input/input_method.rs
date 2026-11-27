use std::ops::Range;

use gpui::{Bounds, Context, EntityInputHandler, Pixels, Point, UTF16Selection, Window};

use crate::control::editor::text_input::TextInput;

type IgnoreDisabledInput = bool;

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

    // PLATFORM: linux a press unmarks, and GTK's input method drops the preedit; macOS commits it.
    fn unmark_text(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if cfg!(target_os = "linux") && self.buffer.marked().is_some() {
            self.replace_text_in_range(None, "", window, cx);
        }
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
mod tests {
    use gpui::{EntityInputHandler as _, TestAppContext};

    use crate::control::editor::test_support::field::{field, text_of};

    #[gpui::test]
    fn composition_is_marked_and_replaced_by_the_committed_text(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        let marked = input.update_in(cx, |input, window, cx| {
            input.replace_and_mark_text_in_range(None, "ni", None, window, cx);
            input.marked_text_range(window, cx)
        });
        assert_eq!(marked, Some(0..2));
        assert_eq!(text_of(&input, cx), "ni");
        input.update_in(cx, |input, window, cx| {
            input.replace_text_in_range(None, "你", window, cx);
        });
        assert_eq!(text_of(&input, cx), "你");
    }

    #[gpui::test]
    fn ranges_are_reported_in_utf16_units(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("a😀b");
        let selection = input.update_in(cx, |input, window, cx| {
            input.selected_text_range(false, window, cx)
        });
        assert_eq!(selection.map(|selection| selection.range), Some(4..4));
        let slice = input.update_in(cx, |input, window, cx| {
            input.text_for_range(1..3, &mut None, window, cx)
        });
        assert_eq!(slice.as_deref(), Some("😀"));
    }

    #[cfg(target_os = "linux")]
    #[gpui::test]
    fn a_press_drops_the_composition_and_keeps_the_typed_text(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("ab");
        input.update_in(cx, |input, window, cx| {
            input.replace_and_mark_text_in_range(None, "ni", None, window, cx);
            input.unmark_text(window, cx);
        });
        assert_eq!(text_of(&input, cx), "ab");
    }
}
