//! Finds the link under a cell.

use alacritty_terminal::event::EventListener;
use alacritty_terminal::grid::Dimensions;
use alacritty_terminal::index::{Column, Line, Point};
use alacritty_terminal::term::Term;

use crate::highlight::Highlight;
use crate::link::Link;
use crate::link_target::LinkTarget;
use crate::link_token::LinkToken;
use crate::link_tokens::tokenize;
use crate::logical_line::LogicalLine;
use crate::viewport::Viewport;

pub(crate) fn find_link<T: EventListener>(
    term: &Term<T>,
    viewport: &Viewport,
    line: Line,
    column: usize,
) -> Option<Link> {
    hyperlink_at(term, viewport, line, column)
        .or_else(|| detected_link_at(term, viewport, line, column))
}

fn hyperlink_at<T: EventListener>(
    term: &Term<T>,
    viewport: &Viewport,
    line: Line,
    column: usize,
) -> Option<Link> {
    let hovered = term.grid()[Point::new(line, Column(column))].hyperlink()?;
    let same = |at: usize| {
        term.grid()[Point::new(line, Column(at))]
            .hyperlink()
            .as_ref()
            == Some(&hovered)
    };
    let start = (0..column)
        .rev()
        .take_while(|at| same(*at))
        .last()
        .unwrap_or(column);
    let end = (column + 1..term.columns())
        .take_while(|at| same(*at))
        .last()
        .unwrap_or(column)
        + 1;
    let row = viewport.row_of(line)?;
    let cell = Highlight {
        row,
        start_column: start,
        end_column: end,
    };
    Some(Link {
        target: LinkTarget::Url(hovered.uri().to_owned()),
        cells: vec![cell],
    })
}

fn detected_link_at<T: EventListener>(
    term: &Term<T>,
    viewport: &Viewport,
    line: Line,
    column: usize,
) -> Option<Link> {
    let logical = LogicalLine::around(term, line);
    let offset = logical.offset_of(line, column)?;
    let covers = |token: &LinkToken| token.start <= offset && offset < token.end;
    let token = tokenize(&logical.text).into_iter().find(covers)?;
    Some(Link {
        cells: token_cells(&logical, &token, viewport),
        target: token.target,
    })
}

fn token_cells(
    logical: &LogicalLine,
    token: &LinkToken,
    viewport: &Viewport,
) -> Vec<Highlight> {
    let covered = logical
        .cells
        .get(token.start..token.end)
        .unwrap_or_default();
    let visible = covered
        .iter()
        .filter_map(|cell| Some((viewport.row_of(cell.line)?, cell)));
    visible.fold(Vec::new(), |mut cells, (row, cell)| {
        extend_or_push(&mut cells, row, cell.column, cell.column + cell.width);
        cells
    })
}

fn extend_or_push(
    cells: &mut Vec<Highlight>,
    row: usize,
    start_column: usize,
    end_column: usize,
) {
    if let Some(last) = cells.last_mut().filter(|last| last.row == row) {
        last.end_column = last.end_column.max(end_column);
        return;
    }
    cells.push(Highlight {
        row,
        start_column,
        end_column,
    });
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::snapshot_builder::viewport_of;
    use crate::test_support::emulator_showing;

    fn link_in(
        output: &str,
        column: usize,
    ) -> Option<Link> {
        let term = emulator_showing(output, 30, 4);
        find_link(&term, &viewport_of(&term), Line(0), column)
    }

    #[test]
    fn urls_and_files_are_found_under_the_pointer_only() {
        let url = link_in("go to https://x.dev/a now", 9);
        assert_eq!(
            url.map(|link| link.target),
            Some(LinkTarget::Url("https://x.dev/a".into()))
        );
        let file = link_in("at src/main.rs:12", 5);
        let expected = LinkTarget::File {
            path: "src/main.rs".into(),
            line: Some(12),
        };
        assert_eq!(file.map(|link| link.target), Some(expected));
        assert!(link_in("plain words", 2).is_none());
    }

    #[test]
    fn the_link_covers_its_cells() {
        let link = link_in("x https://a.dev/b", 4);
        let cells = link.map_or_else(Vec::new, |link| link.cells);
        assert_eq!(
            cells
                .first()
                .map(|cell| (cell.start_column, cell.end_column)),
            Some((2, 17))
        );
    }
}
