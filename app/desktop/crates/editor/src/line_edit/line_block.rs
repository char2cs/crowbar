use std::ops::Range;

use super::line_table::LineTable;
use crate::Vertical;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct LineBlock {
    pub(crate) first: usize,
    pub(crate) last: usize,
}

impl LineBlock {
    pub(crate) fn merged(
        table: &LineTable<'_>,
        selections: &[Range<usize>],
    ) -> Vec<Self> {
        let mut blocks: Vec<Self> = selections
            .iter()
            .map(|selection| Self::of(table, selection))
            .collect();
        blocks.sort_by_key(|block| block.first);
        blocks.into_iter().fold(Vec::new(), Self::joined)
    }

    fn of(
        table: &LineTable<'_>,
        selection: &Range<usize>,
    ) -> Self {
        let first = table.line_of(selection.start);
        let end_line = table.line_of(selection.end);
        let ends_at_line_start = table.start(end_line) == selection.end;
        let excludes_end_line = ends_at_line_start && end_line > first;
        let last = end_line - usize::from(excludes_end_line);
        Self { first, last }
    }

    fn joined(
        mut blocks: Vec<Self>,
        block: Self,
    ) -> Vec<Self> {
        let Some(previous) = blocks.last_mut() else {
            blocks.push(block);
            return blocks;
        };
        if block.first > previous.last + 1 {
            blocks.push(block);
            return blocks;
        }
        previous.last = previous.last.max(block.last);
        blocks
    }

    pub(crate) fn lines(self) -> std::ops::RangeInclusive<usize> {
        self.first..=self.last
    }

    pub(crate) fn contains(
        self,
        line: usize,
    ) -> bool {
        self.lines().contains(&line)
    }

    pub(crate) fn moved_lines(
        self,
        from: usize,
        toward: Vertical,
    ) -> Vec<usize> {
        match toward {
            Vertical::Up => (from..self.first - 1)
                .chain(self.lines())
                .chain(std::iter::once(self.first - 1))
                .collect(),
            Vertical::Down => (from..self.first)
                .chain(std::iter::once(self.last + 1))
                .chain(self.lines())
                .collect(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case<'a> = (&'a [Range<usize>], &'a [(usize, usize)]);

    #[test]
    fn selections_cover_whole_lines_and_touching_blocks_merge() {
        let table = LineTable::of("a\nb\nc\nd\ne");
        let cases: [Case<'_>; 3] = [
            (&[0..0, 4..4], &[(0, 0), (2, 2)]),
            (&[0..0, 2..2], &[(0, 1)]),
            (std::slice::from_ref(&(2..6)), &[(1, 2)]),
        ];
        for (selections, expected) in cases {
            let blocks: Vec<(usize, usize)> = LineBlock::merged(&table, selections)
                .iter()
                .map(|block| (block.first, block.last))
                .collect();
            assert_eq!(blocks, expected, "{selections:?}");
        }
    }
}
