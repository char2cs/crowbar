//! Caret movement, deletion and clipboard commands of [`TextInput`].

use std::ops::Range;

use gpui::{ClipboardItem, Context, EntityInputHandler as _, Window};

use crate::direction::Direction;
use crate::text_input::TextInput;

impl TextInput {
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

    fn collapse_edge(
        selection: &Range<usize>,
        direction: Direction,
    ) -> usize {
        match direction {
            Direction::Forward => selection.end,
            Direction::Backward => selection.start,
        }
    }

    fn caret_target(
        &self,
        selection: &Range<usize>,
        direction: Direction,
    ) -> usize {
        if selection.is_empty() {
            return self.boundary(direction);
        }
        Self::collapse_edge(selection, direction)
    }

    pub(crate) fn move_caret(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        let selection = self.buffer.selection();
        let target = self.caret_target(&selection, direction);
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

    pub(crate) fn erase(
        &mut self,
        direction: Direction,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.buffer.selection().is_empty() {
            self.buffer.select_to(self.boundary(direction));
        }
        self.replace_text_in_range(None, "", window, cx);
    }

    pub(crate) fn paste(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(text) = cx.read_from_clipboard().and_then(|item| item.text()) else {
            return;
        };
        self.replace_text_in_range(None, &text.replace('\n', " "), window, cx);
    }

    pub(crate) fn cut(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.copy(cx);
        self.replace_text_in_range(None, "", window, cx);
    }

    pub(crate) fn copy(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.buffer.selection().is_empty() {
            return;
        }
        cx.write_to_clipboard(ClipboardItem::new_string(
            self.buffer.selected_text().to_owned(),
        ));
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::test_support::{field, text_of};

    #[gpui::test]
    fn backspace_removes_a_whole_grapheme(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("ae\u{301}");
        cx.simulate_keystrokes("backspace");
        assert_eq!(text_of(&input, cx), "a");
    }

    #[gpui::test]
    fn delete_removes_the_grapheme_after_the_caret(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("abc");
        cx.simulate_keystrokes("home delete");
        assert_eq!(text_of(&input, cx), "bc");
    }

    #[gpui::test]
    fn arrows_move_the_caret_and_selection_collapses_to_its_edge(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("abc");
        cx.simulate_keystrokes("left left x");
        assert_eq!(text_of(&input, cx), "axbc");
        cx.simulate_keystrokes("shift-right shift-right left y");
        assert_eq!(text_of(&input, cx), "axybc");
    }

    #[gpui::test]
    fn select_all_then_typing_replaces_everything(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("abc");
        cx.simulate_keystrokes("ctrl-a");
        cx.simulate_input("z");
        assert_eq!(text_of(&input, cx), "z");
    }

    #[gpui::test]
    fn cut_copy_and_paste_round_trip_through_the_clipboard(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("hello");
        cx.simulate_keystrokes("ctrl-a ctrl-x");
        assert_eq!(text_of(&input, cx), "");
        cx.simulate_keystrokes("ctrl-v ctrl-v");
        assert_eq!(text_of(&input, cx), "hellohello");
    }

    #[gpui::test]
    fn cut_without_a_selection_deletes_nothing(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("abc");
        cx.simulate_keystrokes("ctrl-x");
        assert_eq!(text_of(&input, cx), "abc");
    }
}
