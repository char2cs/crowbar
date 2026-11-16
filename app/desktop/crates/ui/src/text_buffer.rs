//! The editable text, selection and composition state of a single-line input.

use std::ops::Range;

use unicode_segmentation::UnicodeSegmentation;

use crate::direction::Direction;
use crate::text_words;

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

    pub(crate) fn previous_boundary(
        &self,
        offset: usize,
    ) -> usize {
        self.text
            .grapheme_indices(true)
            .rev()
            .find_map(|(index, _)| (index < offset).then_some(index))
            .unwrap_or(0)
    }

    pub(crate) fn next_boundary(
        &self,
        offset: usize,
    ) -> usize {
        self.text
            .grapheme_indices(true)
            .find_map(|(index, _)| (index > offset).then_some(index))
            .unwrap_or(self.text.len())
    }

    pub(crate) fn word_boundary(
        &self,
        direction: Direction,
    ) -> usize {
        let from = self.cursor();
        match direction {
            Direction::Forward => text_words::next_end(&self.text, from),
            Direction::Backward => text_words::previous_start(&self.text, from),
        }
    }

    pub(crate) fn press(
        &mut self,
        offset: usize,
        (clicks, extend): (usize, bool),
    ) {
        let offset = self.clamp(offset);
        let span = match clicks {
            0 | 1 => None,
            2 => Some(text_words::around(&self.text, offset)),
            _ => Some(text_words::line_around(&self.text, offset)),
        };
        match (span, extend) {
            (Some(span), _) => {
                self.selection = span;
                self.reversed = false;
            },
            (None, true) => self.select_to(offset),
            (None, false) => self.move_to(offset),
        }
    }

    pub(crate) fn erase_toward(
        &mut self,
        edge: usize,
    ) {
        if self.selection.is_empty() {
            self.select_to(edge);
        }
        self.replace(None, "");
    }

    pub(crate) fn replace(
        &mut self,
        range: Option<Range<usize>>,
        new_text: &str,
    ) {
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

    pub(crate) fn utf16_offset(
        &self,
        offset: usize,
    ) -> usize {
        self.text
            .get(..offset)
            .map_or(0, |head| head.encode_utf16().count())
    }

    pub(crate) fn utf8_offset(
        &self,
        offset: usize,
    ) -> usize {
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

    pub(crate) fn range_to_utf16(
        &self,
        range: &Range<usize>,
    ) -> Range<usize> {
        self.utf16_offset(range.start)..self.utf16_offset(range.end)
    }

    pub(crate) fn range_from_utf16(
        &self,
        range: &Range<usize>,
    ) -> Range<usize> {
        self.utf8_offset(range.start)..self.utf8_offset(range.end)
    }

    fn target(
        &self,
        range: Option<Range<usize>>,
    ) -> Range<usize> {
        let range = range
            .or_else(|| self.marked.clone())
            .unwrap_or_else(|| self.selection.clone());
        self.clamp(range.start)..self.clamp(range.end)
    }

    fn clamp(
        &self,
        offset: usize,
    ) -> usize {
        let mut offset = offset.min(self.text.len());
        while !self.text.is_char_boundary(offset) {
            offset -= 1;
        }
        offset
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn typing_replaces_the_selection_and_moves_the_caret() {
        let mut buffer = TextBuffer::with_text("hello world");
        buffer.move_to(0);
        buffer.select_to(5);
        buffer.replace(None, "bye");
        assert_eq!(buffer.text(), "bye world");
        assert_eq!(buffer.selection(), 3..3);
    }

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
    fn boundaries_respect_graphemes() {
        let buffer = TextBuffer::with_text("a\u{1F468}\u{200D}\u{1F469}b");
        let family_end = buffer.next_boundary(1);
        assert_eq!(
            buffer.text().get(1..family_end),
            Some("\u{1F468}\u{200D}\u{1F469}")
        );
        assert_eq!(buffer.previous_boundary(family_end), 1);
        assert_eq!(
            buffer.next_boundary(buffer.text().len()),
            buffer.text().len()
        );
        assert_eq!(buffer.previous_boundary(0), 0);
    }

    #[test]
    fn utf16_offsets_round_trip() {
        const CASES: [(usize, usize); 5] = [(0, 0), (1, 1), (5, 3), (7, 4), (8, 5)];
        let buffer = TextBuffer::with_text("a\u{1F600}\u{e9}z");
        for (utf8, utf16) in CASES {
            assert_eq!(buffer.utf16_offset(utf8), utf16, "to {utf8}");
            assert_eq!(buffer.utf8_offset(utf16), utf8, "from {utf16}");
        }
    }

    #[test]
    fn composition_is_replaced_until_committed() {
        let mut buffer = TextBuffer::with_text("");
        buffer.replace_and_mark(None, "ni", None);
        assert_eq!(buffer.marked(), Some(0..2));
        buffer.replace_and_mark(None, "\u{4f60}", None);
        assert_eq!((buffer.text(), buffer.marked()), ("\u{4f60}", Some(0..3)));
        buffer.replace(None, "\u{4f60}");
        assert_eq!(
            (buffer.text(), buffer.marked(), buffer.selection()),
            ("\u{4f60}", None, 3..3)
        );
    }

    #[test]
    fn out_of_range_offsets_are_clamped_to_char_boundaries() {
        let mut buffer = TextBuffer::with_text("\u{e9}a");
        buffer.move_to(1);
        assert_eq!(buffer.cursor(), 0);
        buffer.replace(Some(0..99), "x");
        assert_eq!(buffer.text(), "x");
    }

    #[test]
    fn erasing_removes_the_selection_or_the_span_to_the_edge() {
        type Case = (&'static str, usize, usize, usize, &'static str);
        const CASES: &[Case] = &[
            ("abc", 1, 1, 0, "bc"),
            ("abc", 1, 1, 2, "ac"),
            ("abc", 0, 2, 3, "c"),
        ];
        for &(text, from, to, edge, want) in CASES {
            let mut buffer = TextBuffer::with_text(text);
            buffer.move_to(from);
            buffer.select_to(to);
            buffer.erase_toward(edge);
            assert_eq!(buffer.text(), want, "{text} {from}..{to} edge {edge}");
        }
    }

    #[test]
    fn presses_place_the_caret_select_a_word_or_a_line_and_extend_on_shift() {
        type Case = (usize, usize, bool, Range<usize>);
        let cases: &[Case] = &[
            (5, 1, false, 5..5),
            (5, 1, true, 4..5),
            (5, 2, false, 4..7),
            (5, 3, false, 0..7),
            (9, 3, true, 8..11),
        ];
        for (offset, clicks, extend, want) in cases {
            let mut buffer = TextBuffer::with_text("one two\nsix");
            buffer.move_to(4);
            buffer.press(*offset, (*clicks, *extend));
            assert_eq!(buffer.selection(), *want, "{offset} x{clicks} {extend}");
        }
    }

    #[test]
    fn word_boundaries_start_from_the_caret() {
        let mut buffer = TextBuffer::with_text("one two");
        buffer.move_to(4);
        assert_eq!(buffer.word_boundary(Direction::Backward), 0);
        assert_eq!(buffer.word_boundary(Direction::Forward), 7);
    }

    #[test]
    fn select_all_covers_the_text() {
        let mut buffer = TextBuffer::with_text("abc");
        buffer.select_all();
        assert_eq!(buffer.selected_text(), "abc");
    }
}
