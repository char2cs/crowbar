//! Caret movement, deletion and clipboard commands of [`TextInput`].

use std::ops::Range;

use gpui::{ClipboardItem, Context, EntityInputHandler as _, Window};

use crate::direction::Direction;
use crate::text_input::TextInput;

impl TextInput {
    fn boundary(&self, direction: Direction) -> usize {
        let from = self.buffer.cursor();
        match direction {
            Direction::Forward => self.buffer.next_boundary(from),
            Direction::Backward => self.buffer.previous_boundary(from),
        }
    }

    fn collapse_edge(selection: &Range<usize>, direction: Direction) -> usize {
        match direction {
            Direction::Forward => selection.end,
            Direction::Backward => selection.start,
        }
    }

    pub(crate) fn move_caret(&mut self, direction: Direction, cx: &mut Context<'_, Self>) {
        let selection = self.buffer.selection();
        let target = if selection.is_empty() {
            self.boundary(direction)
        } else {
            Self::collapse_edge(&selection, direction)
        };
        self.buffer.move_to(target);
        cx.notify();
    }

    pub(crate) fn extend_selection(&mut self, direction: Direction, cx: &mut Context<'_, Self>) {
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

    pub(crate) fn paste(&mut self, window: &mut Window, cx: &mut Context<'_, Self>) {
        let Some(text) = cx.read_from_clipboard().and_then(|item| item.text()) else {
            return;
        };
        self.replace_text_in_range(None, &text.replace('\n', " "), window, cx);
    }

    pub(crate) fn cut(&mut self, window: &mut Window, cx: &mut Context<'_, Self>) {
        self.copy(cx);
        self.replace_text_in_range(None, "", window, cx);
    }

    pub(crate) fn copy(&self, cx: &mut Context<'_, Self>) {
        if self.buffer.selection().is_empty() {
            return;
        }
        cx.write_to_clipboard(ClipboardItem::new_string(
            self.buffer.selected_text().to_owned(),
        ));
    }
}

#[cfg(test)]
#[path = "text_input_motion_tests.rs"]
mod tests;
