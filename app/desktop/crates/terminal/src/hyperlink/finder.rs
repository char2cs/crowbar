//! Finds the link under a cell among the OSC 8 hyperlinks and the URLs and paths in the text.

use std::ops::Range;

use alacritty_terminal::event::EventListener;
use alacritty_terminal::grid::Dimensions;
use alacritty_terminal::index::{Column, Line, Point};
use alacritty_terminal::term::Term;

use crate::hyperlink::link::Link;
use crate::hyperlink::link_target::LinkTarget;
use crate::hyperlink::link_token::LinkToken;
use crate::hyperlink::logical_line::LogicalLine;
use crate::hyperlink::scan::PathMatch;
use crate::hyperlink::scan::address_runs;
use crate::hyperlink::scan::find_paths;
use crate::hyperlink::scan::find_urls;
use crate::snapshot::Highlight;
use crate::snapshot::Viewport;

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

fn tokenize(text: &str) -> Vec<LinkToken> {
    let runs = address_runs(text);
    let urls = find_urls(text)
        .into_iter()
        .filter_map(|span| url_token(text, span));
    let files = find_paths(text)
        .into_iter()
        .filter_map(|found| file_token(text, &found, &runs));
    urls.chain(files).collect()
}

fn url_token(
    text: &str,
    span: Range<usize>,
) -> Option<LinkToken> {
    let address = text.get(span.clone())?.to_owned();
    Some(LinkToken {
        target: LinkTarget::Url(address),
        start: span.start,
        end: span.end,
    })
}

fn file_token(
    text: &str,
    found: &PathMatch,
    runs: &[Range<usize>],
) -> Option<LinkToken> {
    let inside_address = runs.iter().any(|run| run.contains(&found.span.start));
    if inside_address {
        return None;
    }
    let path = text.get(found.span.start..found.path_end)?.to_owned();
    let target = LinkTarget::File {
        path,
        line: found.line,
    };
    Some(LinkToken {
        target,
        start: found.span.start,
        end: found.span.end,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::terminal::snapshot_builder::viewport_of;
    use crate::test_support::emulator_showing;

    type Found = (LinkTarget, Vec<(usize, usize)>);

    fn link_in(
        output: &str,
        column: usize,
    ) -> Option<Found> {
        let term = emulator_showing(output, 30, 4);
        let link = find_link(&term, &viewport_of(&term), Line(0), column)?;
        let cells = link
            .cells
            .iter()
            .map(|cell| (cell.start_column, cell.end_column));
        Some((link.target, cells.collect()))
    }

    #[test]
    fn urls_and_files_are_found_under_the_pointer_only() {
        let url = |address: &str| LinkTarget::Url(address.into());
        let file = LinkTarget::File {
            path: "src/main.rs".into(),
            line: Some(12),
        };
        let hyperlink = "\x1b]8;;https://h.dev\x1b\\docs\x1b]8;;\x1b\\ x";
        let cases = [
            (
                "go to https://x.dev/a now",
                9,
                Some((url("https://x.dev/a"), vec![(6, 21)])),
            ),
            ("at src/main.rs:12", 5, Some((file, vec![(3, 17)]))),
            ("www.x.com/y", 2, None),
            ("plain words", 2, None),
            (hyperlink, 1, Some((url("https://h.dev"), vec![(0, 4)]))),
        ];
        for (output, column, expected) in cases {
            assert_eq!(link_in(output, column), expected, "{output:?}");
        }
    }
}
