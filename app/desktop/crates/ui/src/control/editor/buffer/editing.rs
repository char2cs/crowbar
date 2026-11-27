use std::ops::Range;

use super::Buffer;

impl Buffer {
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

    pub(crate) fn target(
        &self,
        range: Option<Range<usize>>,
    ) -> Range<usize> {
        let range = range
            .or_else(|| self.marked.clone())
            .unwrap_or_else(|| self.selection.clone());
        self.clamp(range.start)..self.clamp(range.end)
    }

    pub(crate) fn clamp(
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
    fn composition_is_replaced_until_committed() {
        let mut buffer = Buffer::with_text("");
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
        let mut buffer = Buffer::with_text("\u{e9}a");
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
            let mut buffer = Buffer::with_text(text);
            buffer.move_to(from);
            buffer.select_to(to);
            buffer.erase_toward(edge);
            assert_eq!(buffer.text(), want, "{text} {from}..{to} edge {edge}");
        }
    }

    #[test]
    fn typing_replaces_the_selection_and_moves_the_caret() {
        let mut buffer = Buffer::with_text("hello world");
        buffer.move_to(0);
        buffer.select_to(5);
        buffer.replace(None, "bye");
        assert_eq!(buffer.text(), "bye world");
        assert_eq!(buffer.selection(), 3..3);
    }
}
