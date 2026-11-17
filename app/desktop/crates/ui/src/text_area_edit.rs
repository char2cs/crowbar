//! Insertion, deletion, clipboard and undo commands of [`TextArea`].

use std::ops::Range;

use gpui::{ClipboardItem, Context};

use crate::direction::Direction;
use crate::edit::Edit;
use crate::text_area::TextArea;
use crate::text_buffer::TextBuffer;

pub(crate) fn edit_kind(
    text: &str,
    selection: &Range<usize>,
) -> Edit {
    let single = text.chars().count() == 1 && text != "\n";
    if single && selection.is_empty() {
        return Edit::Typing;
    }
    Edit::Other
}

fn normalised(text: &str) -> String {
    text.replace("\r\n", "\n").replace('\r', "\n")
}

impl TextArea {
    pub(crate) fn edit(
        &mut self,
        kind: Edit,
        cx: &mut Context<'_, Self>,
        change: impl FnOnce(&mut TextBuffer),
    ) {
        if !self.accepts_input() {
            return;
        }
        let before = self.buffer.clone();
        change(&mut self.buffer);
        if before.text() == self.buffer.text() {
            return;
        }
        if before.marked().is_none() {
            self.history.record(&before, kind);
        }
        self.changed(cx);
    }

    pub(crate) fn insert(
        &mut self,
        text: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let kind = edit_kind(text, &self.buffer.selection());
        self.edit(kind, cx, |buffer| buffer.replace(None, text));
    }

    pub(crate) fn erase(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        let edge = match direction {
            Direction::Forward => self.buffer.next_boundary(self.buffer.cursor()),
            Direction::Backward => self.buffer.previous_boundary(self.buffer.cursor()),
        };
        self.edit(Edit::Other, cx, |buffer| buffer.erase_toward(edge));
    }

    pub(crate) fn erase_word(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        let edge = self.buffer.word_boundary(direction);
        self.edit(Edit::Other, cx, |buffer| buffer.erase_toward(edge));
    }

    pub(crate) fn paste(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(text) = cx.read_from_clipboard().and_then(|item| item.text()) else {
            return;
        };
        let text = normalised(&text);
        self.edit(Edit::Other, cx, |buffer| buffer.replace(None, &text));
    }

    pub(crate) fn cut(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.copy(cx);
        self.edit(Edit::Other, cx, |buffer| buffer.replace(None, ""));
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

    pub(crate) fn undo(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(previous) = self.history.undo(&self.buffer) else {
            return;
        };
        self.restore(previous, cx);
    }

    pub(crate) fn redo(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(next) = self.history.redo(&self.buffer) else {
            return;
        };
        self.restore(next, cx);
    }

    fn restore(
        &mut self,
        snapshot: TextBuffer,
        cx: &mut Context<'_, Self>,
    ) {
        self.buffer = snapshot;
        self.changed(cx);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::{area, area_text};

    #[test]
    fn only_a_lone_character_typed_over_nothing_coalesces() {
        type Case = (&'static str, Range<usize>, Edit);
        const CASES: &[Case] = &[
            ("a", 2..2, Edit::Typing),
            ("a", 1..2, Edit::Other),
            ("\n", 2..2, Edit::Other),
            ("ab", 2..2, Edit::Other),
        ];
        for (text, selection, kind) in CASES {
            assert_eq!(edit_kind(text, selection), *kind, "{text:?} {selection:?}");
        }
    }

    #[test]
    fn line_endings_are_normalised_to_newlines() {
        assert_eq!(normalised("a\r\nb\rc\nd"), "a\nb\nc\nd");
    }

    #[gpui::test]
    fn paste_keeps_newlines_and_cut_copy_round_trip(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        cx.write_to_clipboard(ClipboardItem::new_string("a\r\nb".into()));
        cx.simulate_keystrokes("secondary-v");
        assert_eq!(area_text(&area, cx), "a\nb");
        cx.simulate_keystrokes("secondary-a secondary-x");
        assert_eq!(area_text(&area, cx), "");
        cx.simulate_keystrokes("secondary-v secondary-v");
        assert_eq!(area_text(&area, cx), "a\nba\nb");
    }

    #[gpui::test]
    fn backspace_and_delete_work_across_a_newline(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        cx.simulate_input("a");
        cx.simulate_keystrokes("enter");
        cx.simulate_input("b");
        cx.simulate_keystrokes("left backspace");
        assert_eq!(area_text(&area, cx), "ab");
        cx.simulate_keystrokes("home delete");
        assert_eq!(area_text(&area, cx), "b");
    }

    #[gpui::test]
    fn undo_reverts_a_typed_run_at_once_and_redo_replays_it(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        cx.simulate_input("hello");
        cx.simulate_keystrokes("enter");
        cx.simulate_input("world");
        cx.simulate_keystrokes("secondary-z");
        assert_eq!(area_text(&area, cx), "hello\n");
        cx.simulate_keystrokes("secondary-z");
        assert_eq!(area_text(&area, cx), "hello");
        cx.simulate_keystrokes("secondary-z");
        assert_eq!(area_text(&area, cx), "");
        cx.simulate_keystrokes("secondary-shift-z secondary-y");
        assert_eq!(area_text(&area, cx), "hello\n");
    }

    #[gpui::test]
    fn a_disabled_area_ignores_typing(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| {
            area.set_condition(crate::TextAreaCondition::Disabled, cx);
        });
        cx.simulate_input("x");
        assert_eq!(area_text(&area, cx), "");
    }
}
