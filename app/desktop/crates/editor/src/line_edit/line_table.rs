use std::ops::Range;

const NEWLINE: char = '\n';

pub(crate) struct LineTable<'text> {
    text: &'text str,
    starts: Vec<usize>,
}

impl<'text> LineTable<'text> {
    pub(crate) fn of(text: &'text str) -> Self {
        let breaks = text.match_indices(NEWLINE).map(|(at, _)| at + 1);
        let starts = std::iter::once(0).chain(breaks).collect();
        Self { text, starts }
    }

    pub(crate) fn last(&self) -> usize {
        self.starts.len().saturating_sub(1)
    }

    pub(crate) fn line_of(
        &self,
        offset: usize,
    ) -> usize {
        self.starts
            .partition_point(|start| *start <= offset)
            .saturating_sub(1)
    }

    pub(crate) fn start(
        &self,
        line: usize,
    ) -> usize {
        self.starts.get(line).copied().unwrap_or(self.text.len())
    }

    pub(crate) fn end(
        &self,
        line: usize,
    ) -> usize {
        self.starts
            .get(line + 1)
            .map_or(self.text.len(), |next| next.saturating_sub(1))
    }

    pub(crate) fn line(
        &self,
        line: usize,
    ) -> &'text str {
        self.text
            .get(self.start(line)..self.end(line))
            .unwrap_or("")
    }

    pub(crate) fn span(
        &self,
        lines: Range<usize>,
    ) -> Range<usize> {
        self.start(lines.start)..self.end(lines.end.saturating_sub(1))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn offsets_resolve_to_their_line_and_each_line_s_bounds() {
        let table = LineTable::of("ab\n\ncd");
        let cases = [
            (0, 0, (0, 2), "ab"),
            (3, 1, (3, 3), ""),
            (6, 2, (4, 6), "cd"),
        ];
        for (offset, line, bounds, text) in cases {
            assert_eq!(table.line_of(offset), line, "{offset}");
            assert_eq!((table.start(line), table.end(line)), bounds, "{line}");
            assert_eq!(table.line(line), text, "{line}");
        }
        assert_eq!(table.last(), 2);
    }
}
