use std::ops::Range;

use gpui::{Bounds, FocusHandle, Pixels, SharedString};

use crate::control::editor::field::Editable;
use crate::control::editor::field::Focus;
use crate::control::editor::field::Selection;
use crate::control::editor::field::Text;
use crate::control::editor::text_area::TextArea;
use crate::control::editor::text_area::layout::Layout;
use crate::control::editor::text_area::wrapped_element::Caret;
use crate::control::editor::text_area::wrapped_element::Sink;
use crate::control::editor::text_area::wrapped_element::WrappedSurface;

impl WrappedSurface for TextArea {}

impl Text for TextArea {
    fn content(&self) -> &str {
        self.buffer.text()
    }

    fn hint(&self) -> &SharedString {
        &self.placeholder
    }
}

impl Selection for TextArea {
    fn marked_range(&self) -> Option<Range<usize>> {
        self.buffer.marked()
    }

    fn selected_range(&self) -> Range<usize> {
        self.buffer.selection()
    }
}

impl Caret for TextArea {
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

impl Editable for TextArea {
    fn editable(&self) -> bool {
        self.accepts_input()
    }
}

impl Focus for TextArea {
    fn input_focus(&self) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Sink for TextArea {
    fn record_layout(
        &mut self,
        layout: Layout,
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
