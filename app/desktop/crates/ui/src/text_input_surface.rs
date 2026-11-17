use std::ops::Range;

use gpui::{Bounds, FocusHandle, Pixels, ShapedLine, SharedString};

use crate::field_focus::FieldFocus;
use crate::field_selection::FieldSelection;
use crate::field_text::FieldText;
use crate::text_input::TextInput;
use crate::text_input_layout::TextInputLayout;
use crate::text_surface::TextSurface;

impl TextSurface for TextInput {}

impl FieldText for TextInput {
    fn content(&self) -> &str {
        self.buffer.text()
    }

    fn hint(&self) -> &SharedString {
        &self.placeholder
    }
}

impl FieldSelection for TextInput {
    fn marked_range(&self) -> Option<Range<usize>> {
        self.buffer.marked()
    }

    fn selected_range(&self) -> Range<usize> {
        self.buffer.selection()
    }
}

impl FieldFocus for TextInput {
    fn input_focus(&self) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl TextInputLayout for TextInput {
    fn centered(&self) -> bool {
        self.style.centered()
    }

    fn record_layout(
        &mut self,
        line: ShapedLine,
        bounds: Bounds<Pixels>,
    ) {
        self.layout = Some(line);
        self.bounds = Some(bounds);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::field;

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
