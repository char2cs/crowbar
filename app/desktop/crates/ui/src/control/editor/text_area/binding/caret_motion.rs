//! Caret movement of [`TextArea`] along characters, visual rows and the document.

use gpui::{Context, Pixels};

use crate::control::editor::text_area::TextArea;
use crate::foundation::Direction;

mod edges;

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

    pub(crate) fn move_by_word(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        self.buffer.move_to(self.buffer.word_boundary(direction));
        cx.notify();
    }

    pub(crate) fn extend_by_word(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        self.buffer.select_to(self.buffer.word_boundary(direction));
        cx.notify();
    }

    pub(crate) fn goal_x(&self) -> Pixels {
        let cursor = self.buffer.cursor();
        match self.goal {
            Some((at, x)) if at == cursor => x,
            _ => self.layout.point_for_index(cursor).x,
        }
    }

    pub(crate) fn vertical_target(
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
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::control::editor::test_support::area::{area, area_text};

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
    fn word_keys_move_select_and_delete_by_word_across_rows(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| area.set_text("one two\nthree", cx));
        cx.run_until_parked();
        cx.simulate_keystrokes("ctrl-left ctrl-left");
        cx.simulate_input("X");
        assert_eq!(area_text(&area, cx), "one Xtwo\nthree");
        cx.simulate_keystrokes("ctrl-right ctrl-right ctrl-shift-left");
        cx.simulate_input("Y");
        assert_eq!(area_text(&area, cx), "one Xtwo\nY");
        cx.simulate_keystrokes("alt-left ctrl-delete ctrl-backspace");
        assert_eq!(area_text(&area, cx), "one ");
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
