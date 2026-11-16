//! The terminal emulator: an alacritty grid fed by daemon frames and read as render snapshots.

use std::sync::Arc;

use alacritty_terminal::Term;
use alacritty_terminal::event::Event;
use alacritty_terminal::grid::{Dimensions, Scroll};
use alacritty_terminal::index::{Column, Line, Point, Side};
use alacritty_terminal::selection::{Selection, SelectionType};
use alacritty_terminal::term::TermMode;

use crate::cell_side::CellSide;
use crate::cursor_keys::CursorKeys;
use crate::daemon_frame::DaemonFrame;
use crate::error::Error;
use crate::event_sink::{EventQueue, EventSink};
use crate::feed::Feed;
use crate::grid_size::GridSize;
use crate::key_encoder::encode_key;
use crate::key_input::KeyInput;
use crate::link::Link;
use crate::link_finder::find_link;
use crate::mouse_encoder::encode_mouse;
use crate::mouse_encoding::MouseEncoding;
use crate::mouse_event::MouseEvent;
use crate::mouse_tracking::MouseTracking;
use crate::paste::encode_paste;
use crate::paste_mode::PasteMode;
use crate::screen_buffer::ScreenBuffer;
use crate::search_query::SearchQuery;
use crate::search_state::SearchState;
use crate::search_summary::SearchSummary;
use crate::selection_kind::SelectionKind;
use crate::snapshot::Snapshot;
use crate::snapshot_builder::{build_snapshot, viewport_of};
use crate::terminal_config::TerminalConfig;
use crate::terminal_event::TerminalEvent;
use crate::view_point::ViewPoint;

/// A terminal emulator. It never answers the program itself: the daemon's PTY already does, so queries
/// the emulator would reply to are dropped.
pub struct Terminal {
    term: Term<EventSink>,
    feed: Feed,
    events: EventQueue,
    config: TerminalConfig,
    search: Option<SearchState>,
}

impl std::fmt::Debug for Terminal {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("Terminal")
            .field("size", &self.size())
            .finish_non_exhaustive()
    }
}

impl Terminal {
    /// An empty terminal of the configured size.
    pub fn new(config: TerminalConfig) -> Self {
        let events = EventQueue::default();
        let size = GridSize {
            columns: config.columns,
            lines: config.lines,
        };
        let term = Term::new(
            config.emulator_config(),
            &size,
            EventSink::new(Arc::clone(&events)),
        );
        Self {
            term,
            feed: Feed::default(),
            events,
            config,
            search: None,
        }
    }

    /// Applies a daemon frame and returns what the program asked of the shell.
    pub fn apply(
        &mut self,
        frame: &DaemonFrame,
    ) -> Vec<TerminalEvent> {
        self.apply_batch(std::slice::from_ref(frame))
    }

    /// Applies `frames` in order and rescans an active search once for the whole batch, so fast
    /// output does not rescan the scrollback per frame.
    pub fn apply_batch(
        &mut self,
        frames: &[DaemonFrame],
    ) -> Vec<TerminalEvent> {
        let exits: Vec<i32> = frames
            .iter()
            .filter_map(|frame| self.feed.apply(&mut self.term, frame))
            .collect();
        if let Some(search) = self.search.as_mut() {
            search.refresh(&self.term);
        }
        let mut events = self.drain_events();
        events.extend(exits.into_iter().map(TerminalEvent::Exit));
        events
    }

    fn drain_events(&self) -> Vec<TerminalEvent> {
        let Ok(mut queue) = self.events.lock() else {
            return Vec::new();
        };
        let queued = std::mem::take(&mut *queue);
        queued.into_iter().filter_map(terminal_event).collect()
    }

    /// The grid size in columns and lines.
    pub fn size(&self) -> (usize, usize) {
        (self.term.columns(), self.term.screen_lines())
    }

    /// Resizes the grid; the emulator reflows its lines.
    pub fn resize(
        &mut self,
        columns: usize,
        lines: usize,
    ) {
        self.config.columns = columns;
        self.config.lines = lines;
        self.term.resize(GridSize { columns, lines });
    }

    /// Applies new settings (colours, scrollback, cursor); the grid size is kept.
    pub fn reconfigure(
        &mut self,
        config: TerminalConfig,
    ) {
        let (columns, lines) = self.size();
        self.term.set_options(config.emulator_config());
        self.config = TerminalConfig {
            columns,
            lines,
            ..config
        };
    }

    /// The frame to draw.
    pub fn snapshot(&self) -> Snapshot {
        build_snapshot(&self.term, &self.config.palette, self.search.as_ref())
    }

    /// Scrolls the history by `lines` (positive goes back in time).
    pub fn scroll_lines(
        &mut self,
        lines: i32,
    ) {
        self.term.scroll_display(Scroll::Delta(lines));
    }

    /// Scrolls one screen back in time.
    pub fn scroll_page_up(&mut self) {
        self.term.scroll_display(Scroll::PageUp);
    }

    /// Scrolls one screen forward.
    pub fn scroll_page_down(&mut self) {
        self.term.scroll_display(Scroll::PageDown);
    }

    /// Jumps to the oldest line.
    pub fn scroll_to_top(&mut self) {
        self.term.scroll_display(Scroll::Top);
    }

    /// Jumps to the live screen.
    pub fn scroll_to_bottom(&mut self) {
        self.term.scroll_display(Scroll::Bottom);
    }

    /// Scrolls to exactly `offset` lines above the live screen.
    pub fn set_display_offset(
        &mut self,
        offset: usize,
    ) {
        let current = i64::try_from(self.term.grid().display_offset()).unwrap_or(0);
        let wanted = i64::try_from(offset).unwrap_or(0);
        self.scroll_lines(i32::try_from(wanted - current).unwrap_or(0));
    }

    /// Drops the scrollback and keeps the screen.
    pub fn clear_scrollback(&mut self) {
        self.term.grid_mut().clear_history();
        self.term.selection = None;
    }

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

    /// Drops the selection.
    pub fn clear_selection(&mut self) {
        self.term.selection = None;
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

    /// Searches the scrollback and screen; the first match at or below the top of the view is current.
    ///
    /// # Errors
    ///
    /// Returns [`Error::Pattern`] when a regular-expression query does not compile; the previous search is kept.
    pub fn search(
        &mut self,
        query: SearchQuery,
    ) -> Result<SearchSummary, Error> {
        let top = Line(-i32::try_from(self.term.grid().display_offset()).unwrap_or(0));
        let state = SearchState::start(&self.term, query, top)?;
        let summary = state.summary();
        self.search = Some(state);
        Ok(summary)
    }

    /// Moves to the next match, scrolling it into view.
    pub fn search_next(&mut self) -> SearchSummary {
        self.step_search(SearchState::advance)
    }

    /// Moves to the previous match, scrolling it into view.
    pub fn search_previous(&mut self) -> SearchSummary {
        self.step_search(SearchState::retreat)
    }

    fn step_search(
        &mut self,
        step: fn(&mut SearchState),
    ) -> SearchSummary {
        let Some(state) = self.search.as_mut() else {
            return SearchSummary::default();
        };
        step(state);
        if let Some(found) = state.current_match() {
            self.term.scroll_to_point(*found.start());
        }
        state.summary()
    }

    /// Ends the search and removes its highlights.
    pub fn clear_search(&mut self) {
        self.search = None;
    }

    /// The mouse events the program asked for.
    pub fn mouse_tracking(&self) -> MouseTracking {
        let mode = *self.term.mode();
        if mode.contains(TermMode::MOUSE_MOTION) {
            return MouseTracking::Motion;
        }
        if mode.contains(TermMode::MOUSE_DRAG) {
            return MouseTracking::Drag;
        }
        if mode.contains(TermMode::MOUSE_REPORT_CLICK) {
            return MouseTracking::Click;
        }
        MouseTracking::Off
    }

    /// Which screen buffer shows.
    pub fn screen_buffer(&self) -> ScreenBuffer {
        match self.term.mode().contains(TermMode::ALT_SCREEN) {
            true => ScreenBuffer::Alternate,
            false => ScreenBuffer::Primary,
        }
    }

    /// Whether the program wants focus-in and focus-out reports.
    pub fn reports_focus(&self) -> bool {
        self.term.mode().contains(TermMode::FOCUS_IN_OUT)
    }

    /// Whether the cursor is blinking, by the program's choice or the setting.
    pub fn cursor_blinks(&self) -> bool {
        self.term.cursor_style().blinking
    }

    /// The bytes for a mouse event, or `None` when the program did not ask for it.
    pub fn encode_mouse(
        &self,
        event: &MouseEvent,
    ) -> Option<Vec<u8>> {
        encode_mouse(event, self.mouse_tracking(), self.mouse_encoding())
    }

    fn mouse_encoding(&self) -> MouseEncoding {
        let mode = *self.term.mode();
        if mode.contains(TermMode::SGR_MOUSE) {
            return MouseEncoding::Sgr;
        }
        if mode.contains(TermMode::UTF8_MOUSE) {
            return MouseEncoding::Utf8;
        }
        MouseEncoding::Legacy
    }

    /// The bytes for a key press, or `None` for application shortcuts and keys that type nothing.
    pub fn encode_key(
        &self,
        input: &KeyInput,
    ) -> Option<Vec<u8>> {
        encode_key(input, self.cursor_keys())
    }

    fn cursor_keys(&self) -> CursorKeys {
        match self.term.mode().contains(TermMode::APP_CURSOR) {
            true => CursorKeys::Application,
            false => CursorKeys::Normal,
        }
    }

    /// The bytes for pasted text.
    pub fn encode_paste(
        &self,
        text: &str,
    ) -> Vec<u8> {
        let mode = match self.term.mode().contains(TermMode::BRACKETED_PASTE) {
            true => PasteMode::Bracketed,
            false => PasteMode::Plain,
        };
        encode_paste(text, mode)
    }
}

fn side_of(side: CellSide) -> Side {
    match side {
        CellSide::Left => Side::Left,
        CellSide::Right => Side::Right,
    }
}

const MAX_CLIPBOARD_BYTES: usize = 64 * 1024;

fn terminal_event(event: Event) -> Option<TerminalEvent> {
    match event {
        Event::Title(title) => Some(TerminalEvent::Title(title)),
        Event::ResetTitle => Some(TerminalEvent::ResetTitle),
        Event::Bell => Some(TerminalEvent::Bell),
        Event::ClipboardStore(_, text) => {
            (text.len() <= MAX_CLIPBOARD_BYTES).then_some(TerminalEvent::ClipboardStore(text))
        },
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::cell_side::CellSide;
    use crate::link_target::LinkTarget;
    use crate::modifiers::Modifiers;
    use crate::mouse_action::MouseAction;
    use crate::mouse_button::MouseButton;
    use crate::search_options::SearchOptions;
    use crate::test_support::{FOREGROUND, ansi, empty_terminal, row_text, terminal_showing};

    const COLUMNS: usize = 40;
    const LINES: usize = 6;

    fn shown(output: &str) -> Terminal {
        terminal_showing(output, COLUMNS, LINES)
    }

    fn first_row(terminal: &Terminal) -> String {
        terminal
            .snapshot()
            .rows
            .first()
            .map_or_else(String::new, row_text)
    }

    fn at(column: usize) -> ViewPoint {
        ViewPoint { row: 0, column }
    }

    #[test]
    fn output_snapshots_and_resets_show_on_screen() {
        let mut terminal = shown("old text");
        assert_eq!(first_row(&terminal), "old text");
        terminal.apply(&DaemonFrame::Snapshot(b"new".to_vec()));
        assert_eq!(first_row(&terminal), "new");
    }

    #[test]
    fn escape_sequences_split_across_frames_still_apply() {
        let mut terminal = empty_terminal(COLUMNS, LINES);
        for part in ["\x1b[3", "1mred"] {
            terminal.apply(&DaemonFrame::Output(part.as_bytes().to_vec()));
        }
        let snapshot = terminal.snapshot();
        let span = &snapshot.rows[0].segments[0].spans[0];
        assert_eq!(span.foreground, ansi(1));
        assert_ne!(span.foreground.to_hex(), FOREGROUND);
    }

    #[test]
    fn wide_characters_take_two_cells() {
        let terminal = shown("日本");
        let snapshot = terminal.snapshot();
        let cursor = snapshot.cursor.map(|cell| cell.column);
        assert_eq!(cursor, Some(4));
        assert!(
            snapshot.rows[0]
                .segments
                .iter()
                .all(|segment| segment.glyph_columns == 2)
        );
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
    fn search_counts_and_steps_through_matches() {
        let mut terminal = shown("one two ONE");
        let query = SearchQuery {
            text: "one".into(),
            options: SearchOptions::NONE,
        };
        assert_eq!(
            terminal.search(query).map(|summary| summary.total).ok(),
            Some(2)
        );
        assert_eq!(terminal.search_next().current, 2);
        assert_eq!(terminal.search_previous().current, 1);
        terminal.clear_search();
        assert_eq!(
            terminal.snapshot().matches.len(),
            0,
            "clearing the search drops the highlights"
        );
    }

    #[test]
    fn a_batch_updates_the_search_matches_once_for_all_its_frames() {
        let mut terminal = shown("");
        let query = SearchQuery {
            text: "ab".into(),
            options: SearchOptions::NONE,
        };
        assert_eq!(terminal.search(query).map(|s| s.total).ok(), Some(0));
        let frames = [
            DaemonFrame::Output(b"ab ".to_vec()),
            DaemonFrame::Output(b"ab ".to_vec()),
            DaemonFrame::Exit(0),
        ];
        let events = terminal.apply_batch(&frames);
        assert_eq!(terminal.search_next().total, 2);
        assert_eq!(events, vec![TerminalEvent::Exit(0)]);
    }

    #[test]
    fn a_clipboard_store_over_the_cap_is_dropped() {
        let mut terminal = empty_terminal(COLUMNS, LINES);
        let store =
            |encoded: &str| DaemonFrame::Output(format!("\x1b]52;c;{encoded}\x07").into_bytes());
        let small = terminal.apply(&store("aGk="));
        assert_eq!(small, vec![TerminalEvent::ClipboardStore("hi".into())]);
        let over = "eHh4".repeat((MAX_CLIPBOARD_BYTES + 3) / 3);
        assert_eq!(terminal.apply(&store(&over)), Vec::new());
    }

    #[test]
    fn a_bad_regular_expression_is_an_error() {
        let mut terminal = shown("x");
        let query = SearchQuery {
            text: "(".into(),
            options: SearchOptions::REGEX,
        };
        assert!(terminal.search(query).is_err());
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

    #[test]
    fn programs_choose_the_input_encodings() {
        let mut terminal = shown("");
        let paste = |terminal: &Terminal| terminal.encode_paste("a\nb");
        assert_eq!(paste(&terminal), b"a\rb");
        terminal.apply(&DaemonFrame::Output(
            b"\x1b[?2004h\x1b[?1000h\x1b[?1006h\x1b[?1h".to_vec(),
        ));
        assert_eq!(paste(&terminal), b"\x1b[200~a\rb\x1b[201~");
        let press = MouseEvent {
            button: MouseButton::Left,
            action: MouseAction::Press,
            point: at(2),
            modifiers: Modifiers::NONE,
        };
        assert_eq!(
            terminal.encode_mouse(&press),
            Some(b"\x1b[<0;3;1M".to_vec())
        );
        let up = KeyInput {
            key: "up".into(),
            text: None,
            modifiers: Modifiers::NONE,
        };
        assert_eq!(terminal.encode_key(&up), Some(b"\x1bOA".to_vec()));
    }

    #[test]
    fn titles_and_exit_surface_but_queries_are_never_answered() {
        let mut terminal = empty_terminal(COLUMNS, LINES);
        let events = terminal.apply(&DaemonFrame::Output(b"\x1b]0;hi\x07\x1b[6n\x1b[c".to_vec()));
        assert_eq!(events, vec![TerminalEvent::Title("hi".into())]);
        let exit = terminal.apply(&DaemonFrame::Exit(3));
        assert_eq!(exit, vec![TerminalEvent::Exit(3)]);
    }

    #[test]
    fn bulk_output_fills_the_scrollback_and_resize_reflows() {
        let mut terminal = empty_terminal(COLUMNS, LINES);
        let bulk = "row\r\n".repeat(500);
        terminal.apply(&DaemonFrame::Output(bulk.into_bytes()));
        assert!(terminal.snapshot().history_size > 400);
        terminal.resize(20, 4);
        assert_eq!(terminal.size(), (20, 4));
        terminal.scroll_to_top();
        assert!(terminal.snapshot().display_offset > 0);
        terminal.clear_scrollback();
        assert_eq!(terminal.snapshot().history_size, 0);
    }
}
