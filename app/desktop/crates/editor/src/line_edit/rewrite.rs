use std::ops::Range;

use super::follow::Follow;
use super::line_block::LineBlock;
use super::line_table::LineTable;
use crate::Vertical;

const NEWLINE: &str = "\n";

pub(crate) struct Rewrite<'table, 'text> {
    table: &'table LineTable<'text>,
    span: Range<usize>,
    order: Vec<usize>,
}

impl<'table, 'text> Rewrite<'table, 'text> {
    pub(crate) fn moved(
        table: &'table LineTable<'text>,
        blocks: &[LineBlock],
        toward: Vertical,
    ) -> Option<Self> {
        let first = blocks.first()?;
        let last = blocks.last()?;
        let span = match toward {
            Vertical::Up => first.first.checked_sub(1)?..last.last + 1,
            Vertical::Down => first.first..last.last + 2,
        };
        let skip = usize::from(toward == Vertical::Down);
        let starts =
            std::iter::once(span.start).chain(blocks.iter().map(|block| block.last + 1 + skip));
        let order = blocks
            .iter()
            .zip(starts)
            .flat_map(|(block, from)| block.moved_lines(from, toward))
            .collect();
        Some(Self { table, span, order })
    }

    pub(crate) fn copied(
        table: &'table LineTable<'text>,
        blocks: &[LineBlock],
    ) -> Option<Self> {
        let span = blocks.first()?.first..blocks.last()?.last + 1;
        let copy_after = |line: usize| {
            let ending = blocks.iter().find(|block| block.last == line);
            std::iter::once(line).chain(ending.into_iter().flat_map(|block| block.lines()))
        };
        let order = span.clone().flat_map(copy_after).collect();
        Some(Self { table, span, order })
    }

    pub(crate) fn deleted(
        table: &'table LineTable<'text>,
        blocks: &[LineBlock],
    ) -> Option<Self> {
        let first = blocks.first()?;
        let last = blocks.last()?;
        let span = deleted_span(table, *first, *last);
        let kept = |line: &usize| !blocks.iter().any(|block| block.contains(*line));
        let order = span.clone().filter(kept).collect();
        Some(Self { table, span, order })
    }

    pub(crate) fn placed(
        &self,
        selections: &[Range<usize>],
        follow: Follow,
    ) -> Vec<Range<usize>> {
        selections
            .iter()
            .map(|selection| self.place(selection.start, follow)..self.place_end(selection, follow))
            .collect()
    }

    fn place_end(
        &self,
        selection: &Range<usize>,
        follow: Follow,
    ) -> usize {
        let line = self.table.line_of(selection.end);
        let at_line_start = self.table.start(line) == selection.end;
        let past_its_block = at_line_start && line > self.table.line_of(selection.start);
        if !past_its_block {
            return self.place(selection.end, follow);
        }
        self.place(selection.end - NEWLINE.len(), follow) + NEWLINE.len()
    }

    pub(crate) fn range(&self) -> Range<usize> {
        self.table.span(self.span.clone())
    }

    pub(crate) fn text(&self) -> String {
        let lines: Vec<&str> = self
            .order
            .iter()
            .map(|line| self.table.line(*line))
            .collect();
        lines.join(NEWLINE)
    }

    pub(crate) fn caret_after_delete(
        &self,
        selection: &Range<usize>,
        blocks: &[LineBlock],
    ) -> Range<usize> {
        let line = self.table.line_of(selection.start);
        let column = selection.start - self.table.start(line);
        let block = blocks.iter().find(|block| block.contains(line));
        let target = block.and_then(|block| self.replacement_of(*block));
        let Some((index, target)) = target else {
            let start = self.table.start(self.span.start);
            return start..start;
        };
        let caret = self.start_of(index) + column.min(self.table.line(target).len());
        caret..caret
    }

    fn place(
        &self,
        offset: usize,
        follow: Follow,
    ) -> usize {
        let line = self.table.line_of(offset);
        let column = offset - self.table.start(line);
        let mut positions = self.order.iter().enumerate().filter(|(_, at)| **at == line);
        let found = match follow {
            Follow::First => positions.next(),
            Follow::Last => positions.next_back(),
        };
        let index = found.map_or(0, |(index, _)| index);
        self.start_of(index) + column
    }

    fn start_of(
        &self,
        index: usize,
    ) -> usize {
        let before: usize = self
            .order
            .iter()
            .take(index)
            .map(|line| self.table.line(*line).len() + NEWLINE.len())
            .sum();
        self.table.start(self.span.start) + before
    }

    fn replacement_of(
        &self,
        block: LineBlock,
    ) -> Option<(usize, usize)> {
        let wanted = |line: usize| {
            self.order
                .iter()
                .position(|at| *at == line)
                .map(|index| (index, line))
        };
        let before = block.first.checked_sub(1);
        wanted(block.last + 1).or_else(|| before.and_then(wanted))
    }
}

fn deleted_span(
    table: &LineTable<'_>,
    first: LineBlock,
    last: LineBlock,
) -> Range<usize> {
    if last.last == table.last() {
        return first.first.min(last.first.saturating_sub(1))..last.last + 1;
    }
    first.first..last.last + 2
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_delete_takes_the_line_break_after_a_block_or_before_the_last_line() {
        let cases = [("a\nb\nc", 1, 2..5, "c"), ("a\nb\nc", 2, 2..5, "b")];
        for (text, line, range, kept) in cases {
            let table = LineTable::of(text);
            let rewrite = Rewrite::deleted(&table, &[single(line)]);
            assert_eq!(rewrite.as_ref().map(Rewrite::range), Some(range), "{line}");
            assert_eq!(
                rewrite.as_ref().map(Rewrite::text),
                Some(kept.to_owned()),
                "{line}"
            );
        }
    }

    fn single(line: usize) -> LineBlock {
        LineBlock {
            first: line,
            last: line,
        }
    }
}
