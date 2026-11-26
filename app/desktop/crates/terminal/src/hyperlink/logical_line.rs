//! A line of text as the user sees it, with soft-wrapped rows joined.

use alacritty_terminal::event::EventListener;
use alacritty_terminal::grid::Dimensions;
use alacritty_terminal::index::{Column, Line, Point};
use alacritty_terminal::term::Term;
use alacritty_terminal::term::cell::{Cell, Flags};

use crate::hyperlink::cell_ref::CellRef;
use crate::snapshot::cell_columns;

const MAX_WRAPPED_ROWS: usize = 64;

#[derive(Debug, Clone, Default)]
pub(crate) struct LogicalLine {
    pub(crate) text: String,
    pub(crate) cells: Vec<CellRef>,
}

impl LogicalLine {
    pub(crate) fn around<T: EventListener>(
        term: &Term<T>,
        line: Line,
    ) -> Self {
        let top = wrapped_top(term, line);
        let bottom = wrapped_bottom(term, line);
        let mut logical = Self::default();
        for current in top.0..=bottom.0 {
            logical.push_row(term, Line(current));
        }
        logical
    }

    fn push_row<T: EventListener>(
        &mut self,
        term: &Term<T>,
        line: Line,
    ) {
        for column in 0..term.columns() {
            let cell = &term.grid()[Point::new(line, Column(column))];
            self.push_cell(cell, line, column);
        }
    }

    fn push_cell(
        &mut self,
        cell: &Cell,
        line: Line,
        column: usize,
    ) {
        if cell.flags.contains(Flags::WIDE_CHAR_SPACER) {
            return;
        }
        let width = cell_columns(cell.flags);
        let character = match cell.c {
            '\0' => ' ',
            other => other,
        };
        let reference = CellRef {
            line,
            column,
            width,
        };
        self.push_char(character, reference);
        for mark in cell.zerowidth().unwrap_or_default() {
            self.push_char(*mark, reference);
        }
    }

    fn push_char(
        &mut self,
        character: char,
        reference: CellRef,
    ) {
        self.text.push(character);
        self.cells
            .extend(std::iter::repeat_n(reference, character.len_utf8()));
    }

    pub(crate) fn offset_of(
        &self,
        line: Line,
        column: usize,
    ) -> Option<usize> {
        let exact = |cell: &CellRef| {
            cell.line == line && column >= cell.column && column < cell.column + cell.width
        };
        self.cells.iter().position(exact)
    }
}

fn wraps<T: EventListener>(
    term: &Term<T>,
    line: Line,
) -> bool {
    let last = Column(term.columns().saturating_sub(1));
    term.grid()[Point::new(line, last)]
        .flags
        .contains(Flags::WRAPLINE)
}

fn wrapped_top<T: EventListener>(
    term: &Term<T>,
    line: Line,
) -> Line {
    let above = |current: &Line| {
        let previous = Line(current.0 - 1);
        (previous >= term.topmost_line() && wraps(term, previous)).then_some(previous)
    };
    let walked = std::iter::successors(Some(line), above).take(MAX_WRAPPED_ROWS);
    walked.last().unwrap_or(line)
}

fn wrapped_bottom<T: EventListener>(
    term: &Term<T>,
    line: Line,
) -> Line {
    let below = |current: &Line| {
        (*current < term.bottommost_line() && wraps(term, *current)).then_some(Line(current.0 + 1))
    };
    let walked = std::iter::successors(Some(line), below).take(MAX_WRAPPED_ROWS);
    walked.last().unwrap_or(line)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::emulator_showing;

    #[test]
    fn wrapped_rows_join_into_one_line_and_separate_lines_stay_apart() {
        let term = emulator_showing("abcdefghijkl", 5, 4);
        let logical = LogicalLine::around(&term, Line(1));
        assert!(logical.text.starts_with("abcdefghijkl"));
        assert_eq!(logical.offset_of(Line(1), 1), Some(6));
        assert_eq!(logical.offset_of(Line(0), 4), Some(4));
        let term = emulator_showing("ab\r\ncd", 5, 4);
        let logical = LogicalLine::around(&term, Line(1));
        assert!(logical.text.starts_with("cd") && !logical.text.contains("ab"));
    }
}
