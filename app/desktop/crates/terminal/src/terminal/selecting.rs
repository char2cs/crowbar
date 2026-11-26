//! Selecting text and finding links under the pointer.

use alacritty_terminal::grid::Dimensions;
use alacritty_terminal::index::{Column, Point, Side};
use alacritty_terminal::selection::{Selection, SelectionType};

use crate::hyperlink::Link;
use crate::hyperlink::find_link;
use crate::input::CellSide;
use crate::input::SelectionKind;
use crate::input::ViewPoint;
use crate::terminal::Terminal;
use crate::terminal::snapshot_builder::viewport_of;

impl Terminal {
    /// Starts a selection at `point`.
    pub fn begin_selection(
        &mut self,
        point: ViewPoint,
        side: CellSide,
        kind: SelectionKind,
    ) {
        let kind = match kind {
            SelectionKind::Simple => SelectionType::Simple,
            SelectionKind::Word => SelectionType::Semantic,
            SelectionKind::Line => SelectionType::Lines,
        };
        let at = self.grid_point(point);
        self.term.selection = Some(Selection::new(kind, at, side_of(side)));
    }

    /// Extends the selection to `point`.
    pub fn extend_selection(
        &mut self,
        point: ViewPoint,
        side: CellSide,
    ) {
        let at = self.grid_point(point);
        if let Some(selection) = self.term.selection.as_mut() {
            selection.update(at, side_of(side));
        }
    }

    /// Selects the whole scrollback and screen.
    pub fn select_all(&mut self) {
        let start = Point::new(self.term.topmost_line(), Column(0));
        let end = Point::new(self.term.bottommost_line(), self.term.last_column());
        let mut selection = Selection::new(SelectionType::Simple, start, Side::Left);
        selection.update(end, Side::Right);
        self.term.selection = Some(selection);
    }

    /// Ends a drag: an empty selection (a plain click) is dropped; otherwise returns the selected text.
    pub fn finish_selection(&mut self) -> Option<String> {
        let empty = self
            .term
            .selection
            .as_ref()
            .is_some_and(Selection::is_empty);
        if empty {
            self.term.selection = None;
        }
        self.selected_text()
    }

    /// The selected text, when anything is selected.
    pub fn selected_text(&self) -> Option<String> {
        self.term
            .selection_to_string()
            .filter(|text| !text.is_empty())
    }

    fn grid_point(
        &self,
        point: ViewPoint,
    ) -> Point {
        let row = point.row.min(self.term.screen_lines().saturating_sub(1));
        let column = point.column.min(self.term.columns().saturating_sub(1));
        Point::new(viewport_of(&self.term).line_of(row), Column(column))
    }

    /// The link under `point`.
    pub fn link_at(
        &self,
        point: ViewPoint,
    ) -> Option<Link> {
        let at = self.grid_point(point);
        find_link(&self.term, &viewport_of(&self.term), at.line, at.column.0)
    }
}

fn side_of(side: CellSide) -> Side {
    match side {
        CellSide::Left => Side::Left,
        CellSide::Right => Side::Right,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::hyperlink::LinkTarget;
    use crate::test_support::terminal_showing;

    fn shown(output: &str) -> Terminal {
        terminal_showing(output, 40, 6)
    }

    fn at(column: usize) -> ViewPoint {
        ViewPoint { row: 0, column }
    }

    #[test]
    fn selection_kinds_cover_cells_words_and_lines() {
        const CASES: [(SelectionKind, usize, &str); 2] = [
            (SelectionKind::Word, 5, "bar"),
            (SelectionKind::Line, 5, "foo bar baz"),
        ];
        for (kind, column, expected) in CASES {
            let mut terminal = shown("foo bar baz");
            terminal.begin_selection(at(column), CellSide::Left, kind);
            terminal.extend_selection(at(column), CellSide::Right);
            assert_eq!(
                terminal.selected_text().as_deref().map(str::trim_end),
                Some(expected)
            );
        }
    }

    #[test]
    fn a_plain_click_selects_nothing() {
        let mut terminal = shown("foo");
        terminal.begin_selection(at(1), CellSide::Left, SelectionKind::Simple);
        assert_eq!(terminal.finish_selection(), None);
    }

    #[test]
    fn select_all_covers_scrollback_and_screen() {
        let lines = "line\r\n".repeat(20);
        let mut terminal = shown(&lines);
        terminal.select_all();
        let text = terminal.selected_text().unwrap_or_default();
        assert_eq!(text.matches("line").count(), 20, "every line is selected");
    }

    #[test]
    fn urls_in_the_output_are_links() {
        let terminal = shown("see https://x.dev/a now");
        let link = terminal.link_at(at(8));
        assert_eq!(
            link.map(|link| link.target),
            Some(LinkTarget::Url("https://x.dev/a".into()))
        );
        assert!(terminal.link_at(at(0)).is_none());
    }
}
