//! Conversion between UTF-8 byte offsets and the UTF-16 offsets of the platform input protocol.

use std::ops::Range;

use super::TextBuffer;

impl TextBuffer {
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
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn utf16_offsets_round_trip() {
        const CASES: [(usize, usize); 5] = [(0, 0), (1, 1), (5, 3), (7, 4), (8, 5)];
        let buffer = TextBuffer::with_text("a\u{1F600}\u{e9}z");
        for (utf8, utf16) in CASES {
            assert_eq!(buffer.utf16_offset(utf8), utf16, "to {utf8}");
            assert_eq!(buffer.utf8_offset(utf16), utf8, "from {utf16}");
        }
    }
}
