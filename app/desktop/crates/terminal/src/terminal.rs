//! The terminal emulator: an alacritty grid fed by daemon frames and read as render snapshots.

mod event_sink;
mod grid_size;
mod modes;
mod scrolling;
mod searching;
mod selecting;
pub(crate) mod snapshot_builder;
mod terminal_config;
mod terminal_event;
mod title;
mod wire;

pub(crate) use event_sink::{EventQueue, EventSink};
pub(crate) use grid_size::GridSize;
pub use terminal_config::TerminalConfig;
pub use terminal_event::TerminalEvent;
pub use title::sanitize_title;
pub(crate) use wire::Feed;
pub use wire::{ClientMessage, DaemonFrame};

use std::sync::Arc;

use alacritty_terminal::Term;
use alacritty_terminal::event::Event;
use alacritty_terminal::grid::Dimensions;

use crate::find::SearchState;
use crate::snapshot::Snapshot;
use crate::terminal::snapshot_builder::build_snapshot;

/// A terminal emulator. It never answers the program itself: the daemon's PTY already does, so queries
/// the emulator would reply to are dropped.
pub struct Terminal {
    pub(crate) term: Term<EventSink>,
    pub(crate) feed: Feed,
    pub(crate) events: EventQueue,
    pub(crate) config: TerminalConfig,
    pub(crate) search: Option<SearchState>,
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
    fn what_the_program_asks_surfaces_and_queries_are_never_answered() {
        let store =
            |encoded: &str| DaemonFrame::Output(format!("\x1b]52;c;{encoded}\x07").into_bytes());
        let over = "eHh4".repeat((MAX_CLIPBOARD_BYTES + 3) / 3);
        let cases = [
            (
                store("aGk="),
                vec![TerminalEvent::ClipboardStore("hi".into())],
            ),
            (store(&over), Vec::new()),
            (
                DaemonFrame::Output(b"\x1b]0;hi\x07\x1b[6n\x1b[c".to_vec()),
                vec![TerminalEvent::Title("hi".into())],
            ),
            (DaemonFrame::Exit(3), vec![TerminalEvent::Exit(3)]),
        ];
        for (frame, expected) in cases {
            let mut terminal = empty_terminal(COLUMNS, LINES);
            assert_eq!(terminal.apply(&frame), expected);
        }
    }
}
