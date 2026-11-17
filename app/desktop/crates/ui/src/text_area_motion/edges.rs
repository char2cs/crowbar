//! Caret jumps to the edges of a row and of the document.

use std::ops::Range;

use gpui::Context;

use crate::direction::Direction;
use crate::text_area::TextArea;

impl TextArea {
    pub(crate) fn row_edge(
        &self,
        direction: Direction,
    ) -> usize {
        let row: Range<usize> = self.layout.row_bounds(self.buffer.cursor());
        match direction {
            Direction::Backward => row.start,
            Direction::Forward => row.end,
        }
    }

    pub(crate) fn move_to_row_edge(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        self.buffer.move_to(self.row_edge(direction));
        cx.notify();
    }

    pub(crate) fn extend_to_row_edge(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        self.buffer.select_to(self.row_edge(direction));
        cx.notify();
    }

    pub(crate) fn move_to_document_edge(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        match direction {
            Direction::Backward => self.buffer.move_to(0),
            Direction::Forward => self.buffer.move_to(self.buffer.text().len()),
        }
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::test_support::{area, area_text};

    #[gpui::test]
    fn home_end_and_document_edges(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| area.set_text("one\ntwo\nthree", cx));
        cx.run_until_parked();
        cx.simulate_keystrokes("up home");
        cx.simulate_input("[");
        cx.simulate_keystrokes("end");
        cx.simulate_input("]");
        cx.simulate_keystrokes("secondary-home");
        cx.simulate_input("<");
        cx.simulate_keystrokes("secondary-end");
        cx.simulate_input(">");
        assert_eq!(area_text(&area, cx), "<one\n[two]\nthree>");
    }
}
