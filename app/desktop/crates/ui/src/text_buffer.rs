//! The editable text, selection and composition state of a single-line input.

use std::ops::Range;

use unicode_segmentation::UnicodeSegmentation;

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub(crate) struct TextBuffer {
    text: String,
    selection: Range<usize>,
    reversed: bool,
    marked: Option<Range<usize>>,
}

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
            self.selection.start
        } else {
            self.selection.end
        }
    }

    pub(crate) fn set_text(&mut self, text: &str) {
        *self = Self::with_text(text);
    }

    pub(crate) fn move_to(&mut self, offset: usize) {
        let offset = self.clamp(offset);
        self.selection = offset..offset;
        self.reversed = false;
    }

    pub(crate) fn select_to(&mut self, offset: usize) {
        let offset = self.clamp(offset);
        let anchor = if self.reversed {
            self.selection.end
        } else {
            self.selection.start
        };
        self.reversed = offset < anchor;
        self.selection = anchor.min(offset)..anchor.max(offset);
    }

    pub(crate) fn select_all(&mut self) {
        self.selection = 0..self.text.len();
        self.reversed = false;
    }

    pub(crate) fn previous_boundary(&self, offset: usize) -> usize {
        self.text
            .grapheme_indices(true)
            .rev()
            .find_map(|(index, _)| (index < offset).then_some(index))
            .unwrap_or(0)
    }

    pub(crate) fn next_boundary(&self, offset: usize) -> usize {
        self.text
            .grapheme_indices(true)
            .find_map(|(index, _)| (index > offset).then_some(index))
            .unwrap_or(self.text.len())
    }

    pub(crate) fn replace(&mut self, range: Option<Range<usize>>, new_text: &str) {
        let range = self.target(range);
        self.text.replace_range(range.clone(), new_text);
        let caret = range.start + new_text.len();
        self.selection = caret..caret;
        self.reversed = false;
        self.marked = None;
    }

    pub(crate) fn replace_and_mark(
        &mut self,
        range: Option<Range<usize>>,
        new_text: &str,
        selected: Option<Range<usize>>,
    ) {
        let range = self.target(range);
        self.text.replace_range(range.clone(), new_text);
        self.marked = (!new_text.is_empty()).then(|| range.start..range.start + new_text.len());
        let caret = range.start + new_text.len();
        self.selection = selected.map_or(caret..caret, |inner| {
            range.start + inner.start..range.start + inner.end
        });
        self.reversed = false;
    }

    pub(crate) fn unmark(&mut self) {
        self.marked = None;
    }

    pub(crate) fn utf16_offset(&self, offset: usize) -> usize {
        self.text
            .get(..offset)
            .map_or(0, |head| head.encode_utf16().count())
    }

    pub(crate) fn utf8_offset(&self, offset: usize) -> usize {
        let mut units = 0;
        self.text
            .char_indices()
            .find(|(_, character)| {
                let reached = units >= offset;
                units += character.len_utf16();
                reached
            })
            .map_or(self.text.len(), |(index, _)| index)
    }

    pub(crate) fn range_to_utf16(&self, range: &Range<usize>) -> Range<usize> {
        self.utf16_offset(range.start)..self.utf16_offset(range.end)
    }

    pub(crate) fn range_from_utf16(&self, range: &Range<usize>) -> Range<usize> {
        self.utf8_offset(range.start)..self.utf8_offset(range.end)
    }

    fn target(&self, range: Option<Range<usize>>) -> Range<usize> {
        let range = range
            .or_else(|| self.marked.clone())
            .unwrap_or_else(|| self.selection.clone());
        self.clamp(range.start)..self.clamp(range.end)
    }

    fn clamp(&self, offset: usize) -> usize {
        let mut offset = offset.min(self.text.len());
        while !self.text.is_char_boundary(offset) {
            offset -= 1;
        }
        offset
    }
}

#[cfg(test)]
#[path = "text_buffer_tests.rs"]
mod tests;
