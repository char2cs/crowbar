//! Grapheme and word boundaries and the pointer presses built on them.

use unicode_segmentation::UnicodeSegmentation;

use super::Buffer;
use crate::control::editor::buffer::words as text_words;
use crate::foundation::Direction;
use crate::foundation::Pose;

impl Buffer {
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
        (clicks, extend): (usize, Pose),
    ) {
        let offset = self.clamp(offset);
        let span = match clicks {
            0 | 1 => None,
            2 => Some(text_words::around(&self.text, offset)),
            _ => Some(text_words::line_around(&self.text, offset)),
        };
        match (span, extend == Pose::On) {
            (Some(span), _) => {
                self.selection = span;
                self.reversed = false;
            },
            (None, true) => self.select_to(offset),
            (None, false) => self.move_to(offset),
        }
    }
}

#[cfg(test)]
mod tests {
    use std::ops::Range;

    use super::*;

    #[test]
    fn boundaries_respect_graphemes() {
        let buffer = Buffer::with_text("a\u{1F468}\u{200D}\u{1F469}b");
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
    fn word_boundaries_start_from_the_caret() {
        let mut buffer = Buffer::with_text("one two");
        buffer.move_to(4);
        assert_eq!(buffer.word_boundary(Direction::Backward), 0);
        assert_eq!(buffer.word_boundary(Direction::Forward), 7);
    }

    #[test]
    fn presses_place_the_caret_select_a_word_or_a_line_and_extend_on_shift() {
        type Case = (usize, usize, Pose, Range<usize>);
        let cases: &[Case] = &[
            (5, 1, Pose::Off, 5..5),
            (5, 1, Pose::On, 4..5),
            (5, 2, Pose::Off, 4..7),
            (5, 3, Pose::Off, 0..7),
            (9, 3, Pose::On, 8..11),
        ];
        for (offset, clicks, extend, want) in cases {
            let mut buffer = Buffer::with_text("one two\nsix");
            buffer.move_to(4);
            buffer.press(*offset, (*clicks, *extend));
            assert_eq!(buffer.selection(), *want, "{offset} x{clicks} {extend:?}");
        }
    }
}
