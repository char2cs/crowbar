use std::ops::Range;

use gpui::{Bounds, FocusHandle, Pixels, SharedString};

use crate::control::editor::field::FieldEditable;
use crate::control::editor::field::FieldFocus;
use crate::control::editor::field::FieldSelection;
use crate::control::editor::field::FieldText;
use crate::control::editor::text_area::TextArea;
use crate::control::editor::text_area::text_area_element::TextAreaCaret;
use crate::control::editor::text_area::text_area_element::TextAreaSink;
use crate::control::editor::text_area::text_area_element::TextAreaSurface;
use crate::control::editor::text_area::text_area_layout::TextAreaLayout;

impl TextAreaSurface for TextArea {}

impl FieldText for TextArea {
    fn content(&self) -> &str {
        self.buffer.text()
    }

    fn hint(&self) -> &SharedString {
        &self.placeholder
    }
}

impl FieldSelection for TextArea {
    fn marked_range(&self) -> Option<Range<usize>> {
        self.buffer.marked()
    }

    fn selected_range(&self) -> Range<usize> {
        self.buffer.selection()
    }
}

impl TextAreaCaret for TextArea {
    fn cursor(&self) -> usize {
        self.buffer.cursor()
    }

    fn scroll(&self) -> Pixels {
        self.scroll
    }

    fn revealed(&self) -> usize {
        self.revealed
    }
}

impl FieldEditable for TextArea {
    fn editable(&self) -> bool {
        self.accepts_input()
    }
}

impl FieldFocus for TextArea {
    fn input_focus(&self) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl TextAreaSink for TextArea {
    fn record_layout(
        &mut self,
        layout: TextAreaLayout,
        bounds: Bounds<Pixels>,
        scroll: Pixels,
    ) {
        self.layout = layout;
        self.bounds = Some(bounds);
        self.scroll = scroll;
        self.revealed = self.buffer.cursor();
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::control::editor::test_support::area::area;

    #[gpui::test]
    fn the_surface_reports_text_selection_and_caret(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        cx.simulate_input("ab");
        let (content, selected, cursor) = area.read_with(cx, |area, _| {
            (
                area.content().to_owned(),
                area.selected_range(),
                area.cursor(),
            )
        });
        assert_eq!(content, "ab", "content");
        assert_eq!(selected, 2..2, "selection");
        assert_eq!(cursor, 2, "caret");
    }
}
