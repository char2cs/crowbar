//! Caret movement of [`TextArea`] along characters, visual rows and the document.

use std::ops::Range;

use gpui::{Context, Pixels};

use crate::direction::Direction;
use crate::text_area::TextArea;

impl TextArea {
    fn boundary(
        &self,
        direction: Direction,
    ) -> usize {
        let from = self.buffer.cursor();
        match direction {
            Direction::Forward => self.buffer.next_boundary(from),
            Direction::Backward => self.buffer.previous_boundary(from),
        }
    }

    pub(crate) fn move_caret(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        let selection = self.buffer.selection();
        let target = match (selection.is_empty(), direction) {
            (true, _) => self.boundary(direction),
            (false, Direction::Forward) => selection.end,
            (false, Direction::Backward) => selection.start,
        };
        self.buffer.move_to(target);
        cx.notify();
    }

    pub(crate) fn extend_selection(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        self.buffer.select_to(self.boundary(direction));
        cx.notify();
    }

    fn goal_x(&self) -> Pixels {
        let cursor = self.buffer.cursor();
        match self.goal {
            Some((at, x)) if at == cursor => x,
            _ => self.layout.point_for_index(cursor).x,
        }
    }

    fn vertical_target(
        &mut self,
        direction: Direction,
    ) -> usize {
        let goal = self.goal_x();
        let cursor = self.buffer.cursor();
        let target = self.layout.step_vertically(cursor, goal, direction);
        self.goal = Some((target, goal));
        target
    }

    pub(crate) fn move_vertically(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        let target = self.vertical_target(direction);
        self.buffer.move_to(target);
        cx.notify();
    }

    pub(crate) fn extend_vertically(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        let target = self.vertical_target(direction);
        self.buffer.select_to(target);
        cx.notify();
    }

    fn row_edge(
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
    fn up_clamps_to_short_rows_and_remembers_the_wide_column(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| area.set_text("abcdef\nab\nabcdef", cx));
        cx.run_until_parked();
        cx.simulate_keystrokes("up up");
        cx.simulate_input("X");
        assert_eq!(area_text(&area, cx), "abcdefX\nab\nabcdef");
    }

    #[gpui::test]
    fn a_vertical_run_returns_to_the_remembered_column(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| area.set_text("abcdef\nab\nabcdef", cx));
        cx.run_until_parked();
        cx.simulate_keystrokes("left left left up up down down");
        cx.simulate_input("X");
        assert_eq!(area_text(&area, cx), "abcdef\nab\nabcXdef");
    }

    #[gpui::test]
    fn home_end_and_document_edges(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| area.set_text("one\ntwo\nthree", cx));
        cx.run_until_parked();
        cx.simulate_keystrokes("up home");
        cx.simulate_input("[");
        cx.simulate_keystrokes("end");
        cx.simulate_input("]");
        cx.simulate_keystrokes("ctrl-home");
        cx.simulate_input("<");
        cx.simulate_keystrokes("ctrl-end");
        cx.simulate_input(">");
        assert_eq!(area_text(&area, cx), "<one\n[two]\nthree>");
    }

    #[gpui::test]
    fn shift_arrows_select_across_rows(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| area.set_text("ab\ncd", cx));
        cx.run_until_parked();
        cx.simulate_keystrokes("shift-up shift-left");
        cx.simulate_input("X");
        assert_eq!(area_text(&area, cx), "aX");
    }
}
