use std::ops::Range;

use gpui::{Bounds, FocusHandle, Pixels, ShapedLine, SharedString};

use crate::control::editor::field::Blink;
use crate::control::editor::field::Blinking;
use crate::control::editor::field::Focus;
use crate::control::editor::field::Selection;
use crate::control::editor::field::Text;
use crate::control::editor::text_input::TextInput;
use crate::control::editor::text_input::line_element::caret_scroll::CaretScroll;
use crate::control::editor::text_input::line_element::layout::Layout;
use crate::control::editor::text_input::line_element::line_surface::LineSurface;

impl LineSurface for TextInput {}

impl Text for TextInput {
    fn content(&self) -> &str {
        self.buffer.text()
    }

    fn hint(&self) -> &SharedString {
        &self.placeholder
    }
}

impl Selection for TextInput {
    fn marked_range(&self) -> Option<Range<usize>> {
        self.buffer.marked()
    }

    fn selected_range(&self) -> Range<usize> {
        self.buffer.selection()
    }
}

impl Focus for TextInput {
    fn input_focus(&self) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Blinking for TextInput {
    fn blink(&self) -> &Blink {
        &self.blink
    }

    fn blink_mut(&mut self) -> &mut Blink {
        &mut self.blink
    }
}

impl CaretScroll for TextInput {
    fn caret(&self) -> usize {
        self.buffer.cursor()
    }

    fn scrolled(&self) -> Pixels {
        self.scroll
    }
}

impl Layout for TextInput {
    fn centered(&self) -> bool {
        self.style.centered()
    }

    fn record_layout(
        &mut self,
        line: ShapedLine,
        (bounds, scroll): (Bounds<Pixels>, Pixels),
    ) {
        self.layout = Some(line);
        self.bounds = Some(bounds);
        self.scroll = scroll;
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::control::editor::test_support::field::field;

    #[gpui::test]
    fn the_surface_reports_the_typed_text_and_the_caret_selection(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("abc");
        let (content, selected, hint) = input.read_with(cx, |input, _| {
            (
                input.content().to_owned(),
                input.selected_range(),
                input.hint().to_string(),
            )
        });
        assert_eq!(content, "abc", "content");
        assert_eq!(selected, 3..3, "caret at the end");
        assert!(!hint.is_empty(), "hint kept");
    }
}
