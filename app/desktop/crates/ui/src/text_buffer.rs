//! The editable text, selection and composition state of a single-line input.

use std::ops::Range;

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub(crate) struct TextBuffer {
    text: String,
    selection: Range<usize>,
    reversed: bool,
    marked: Option<Range<usize>>,
}

mod boundaries;
mod editing;
mod utf16;

impl TextBuffer {
    pub(crate) fn with_text(text: &str) -> Self {
        Self {
            text: text.to_owned(),
            selection: text.len()..text.len(),
            ..Self::default()
        }
    }

    pub(crate) fn text(&self) -> &str {
        &self.text
    }

    pub(crate) fn selection(&self) -> Range<usize> {
        self.selection.clone()
    }

    pub(crate) fn is_reversed(&self) -> bool {
        self.reversed
    }

    pub(crate) fn marked(&self) -> Option<Range<usize>> {
        self.marked.clone()
    }

    pub(crate) fn selected_text(&self) -> &str {
        self.text.get(self.selection.clone()).unwrap_or_default()
    }

    pub(crate) fn cursor(&self) -> usize {
        if self.reversed {
            return self.selection.start;
        }
        self.selection.end
    }

    pub(crate) fn set_text(
        &mut self,
        text: &str,
    ) {
        *self = Self::with_text(text);
    }

    pub(crate) fn move_to(
        &mut self,
        offset: usize,
    ) {
        let offset = self.clamp(offset);
        self.selection = offset..offset;
        self.reversed = false;
    }

    pub(crate) fn select_to(
        &mut self,
        offset: usize,
    ) {
        let offset = self.clamp(offset);
        let anchor = self.anchor();
        self.reversed = offset < anchor;
        self.selection = anchor.min(offset)..anchor.max(offset);
    }

    fn anchor(&self) -> usize {
        if self.reversed {
            return self.selection.end;
        }
        self.selection.start
    }

    pub(crate) fn select_all(&mut self) {
        self.selection = 0..self.text.len();
        self.reversed = false;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn selection_direction_follows_the_anchor() {
        let mut buffer = TextBuffer::with_text("abcdef");
        buffer.move_to(3);
        buffer.select_to(1);
        assert_eq!(
            (buffer.selection(), buffer.is_reversed(), buffer.cursor()),
            (1..3, true, 1)
        );
        buffer.select_to(5);
        assert_eq!(
            (buffer.selection(), buffer.is_reversed(), buffer.cursor()),
            (3..5, false, 5)
        );
        assert_eq!(buffer.selected_text(), "de");
    }

    #[test]
    fn select_all_covers_the_text() {
        let mut buffer = TextBuffer::with_text("abc");
        buffer.select_all();
        assert_eq!(buffer.selected_text(), "abc");
    }
}
