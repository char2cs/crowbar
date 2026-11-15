//! Everything the view needs to draw one frame.

use crate::cursor_cell::CursorCell;
use crate::highlight::Highlight;
use crate::row::Row;

/// The visible grid, the cursor, and the selection and search overlays, all in viewport coordinates.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Snapshot {
    /// Visible rows, top first.
    pub rows: Vec<Row>,
    /// The cursor, when it shows.
    pub cursor: Option<CursorCell>,
    /// Whether the cursor currently asks to blink.
    pub cursor_blinks: bool,
    /// Selected cells.
    pub selection: Vec<Highlight>,
    /// Search matches other than the current one.
    pub matches: Vec<Highlight>,
    /// The current search match.
    pub current_match: Vec<Highlight>,
    /// Lines the view is scrolled up from the bottom.
    pub display_offset: usize,
    /// Lines of scrollback.
    pub history_size: usize,
}
