use std::ops::Range;

use alacritty_terminal::event::EventListener;
use alacritty_terminal::grid::Dimensions;
use alacritty_terminal::term::{Term, TermMode};
use alacritty_terminal::vte::ansi::CursorShape as EmulatorShape;

use crate::find::SearchState;
use crate::snapshot::ColorSource;
use crate::snapshot::CursorCell;
use crate::snapshot::CursorShape;
use crate::snapshot::Highlight;
use crate::snapshot::Palette;
use crate::snapshot::Snapshot;
use crate::snapshot::Viewport;
use crate::snapshot::{Builder, cell_columns};

pub(crate) fn viewport_of<T: EventListener>(term: &Term<T>) -> Viewport {
    Viewport {
        lines: term.screen_lines(),
        columns: term.columns(),
        display_offset: term.grid().display_offset(),
    }
}

pub(crate) fn build_snapshot<T: EventListener>(
    term: &Term<T>,
    palette: &Palette,
    search: Option<&SearchState>,
) -> Snapshot {
    let viewport = viewport_of(term);
    let colors = ColorSource {
        palette,
        overrides: term.colors(),
    };
    let selection = term
        .selection
        .as_ref()
        .and_then(|selection| selection.to_range(term))
        .map_or_else(Vec::new, |range| {
            viewport.highlights(range.start, range.end)
        });
    let rows = (0..viewport.lines)
        .map(|row| {
            Builder::build(
                colors,
                &term.grid()[viewport.line_of(row)],
                viewport.columns,
                selected_columns(&selection, row),
            )
        })
        .collect();
    Snapshot {
        rows,
        cursor: cursor_of(term, &viewport),
        cursor_blinks: term.cursor_style().blinking,
        selection,
        matches: search.map_or_else(Vec::new, |state| state.others(&viewport)),
        current_match: search.map_or_else(Vec::new, |state| state.current_highlights(&viewport)),
        display_offset: viewport.display_offset,
        history_size: term.history_size(),
    }
}

fn selected_columns(
    selection: &[Highlight],
    row: usize,
) -> Range<usize> {
    selection
        .iter()
        .find(|highlight| highlight.row == row)
        .map_or(0..0, |highlight| {
            highlight.start_column..highlight.end_column
        })
}

fn blank_as_space(character: char) -> char {
    match character {
        '\0' => ' ',
        other => other,
    }
}

fn cursor_of<T: EventListener>(
    term: &Term<T>,
    viewport: &Viewport,
) -> Option<CursorCell> {
    if !term.mode().contains(TermMode::SHOW_CURSOR) {
        return None;
    }
    let shape = match term.cursor_style().shape {
        EmulatorShape::Hidden => return None,
        EmulatorShape::Underline => CursorShape::Underline,
        EmulatorShape::Beam => CursorShape::Bar,
        EmulatorShape::Block | EmulatorShape::HollowBlock => CursorShape::Block,
    };
    let point = term.grid().cursor.point;
    let row = viewport.row_of(point.line)?;
    let cell = &term.grid()[point];
    Some(CursorCell {
        row,
        column: point.column.0,
        columns: cell_columns(cell.flags),
        shape,
        character: blank_as_space(cell.c),
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::snapshot::Rgb;
    use crate::snapshot::{StyleFlags, Underline};
    use crate::test_support::{emulator_showing, row_text};
    use alacritty_terminal::index::{Column, Line, Point, Side};
    use alacritty_terminal::selection::{Selection, SelectionType};

    #[test]
    fn the_snapshot_carries_rows_cursor_and_viewport_size() {
        let term = emulator_showing("hi\r\nthere", 10, 3);
        let viewport = viewport_of(&term);
        assert_eq!((viewport.columns, viewport.lines), (10, 3));
        let reference = Palette {
            foreground: Rgb::from_hex(1),
            background: Rgb::from_hex(2),
            cursor: Rgb::from_hex(3),
            ansi: [Rgb::from_hex(4); 16],
        };
        let snapshot = build_snapshot(&term, &reference, None);
        assert_eq!(
            snapshot.rows.iter().map(row_text).collect::<Vec<_>>(),
            ["hi", "there", ""]
        );
        assert_eq!(
            snapshot.cursor.map(|cell| (cell.row, cell.column)),
            Some((1, 5))
        );
    }

    type Span = (usize, Rgb, StyleFlags, Underline);

    struct Case {
        name: &'static str,
        output: &'static str,
        selected: (usize, usize),
        spans: &'static [Span],
        backgrounds: &'static [usize],
    }

    const FG: Rgb = Rgb::from_hex(1);
    const ANSI: Rgb = Rgb::from_hex(4);

    const CASES: &[Case] = &[
        Case {
            name: "a selected cell draws in the foreground on no background",
            output: "\x1b[41;32mab\x1b[0m",
            selected: (0, 0),
            spans: &[
                (1, FG, StyleFlags::NONE, Underline::None),
                (1, ANSI, StyleFlags::NONE, Underline::None),
            ],
            backgrounds: &[1],
        },
        Case {
            name: "b to e take b's bold and drop c and d's underline, as xterm merges a selected run",
            output: "\x1b[1mab\x1b[0;4mcd\x1b[0mef",
            selected: (1, 4),
            spans: &[
                (5, FG, StyleFlags::BOLD, Underline::None),
                (1, FG, StyleFlags::NONE, Underline::None),
            ],
            backgrounds: &[],
        },
    ];

    #[test]
    fn selected_cells_draw_as_xterm_draws_them() {
        let palette = Palette {
            foreground: FG,
            background: Rgb::from_hex(2),
            cursor: Rgb::from_hex(3),
            ansi: [ANSI; 16],
        };
        let at = |column| Point::new(Line(0), Column(column));
        for case in CASES {
            let mut term = emulator_showing(case.output, 10, 2);
            let mut selection =
                Selection::new(SelectionType::Simple, at(case.selected.0), Side::Left);
            selection.update(at(case.selected.1), Side::Right);
            term.selection = Some(selection);
            let row = &build_snapshot(&term, &palette, None).rows[0];
            let spans: Vec<Span> = row.segments[0]
                .spans
                .iter()
                .map(|span| (span.len, span.foreground, span.flags, span.underline))
                .collect();
            assert_eq!(spans, case.spans, "{}: spans", case.name);
            let filled: Vec<usize> = row.backgrounds.iter().map(|span| span.column).collect();
            assert_eq!(filled, case.backgrounds, "{}: backgrounds", case.name);
        }
    }
}
