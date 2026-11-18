//! Everything the view needs to draw one frame.

mod cursor_cell;
mod cursor_shape;
mod highlight;
mod palette;
mod row;
mod viewport;

pub use cursor_cell::CursorCell;
pub use cursor_shape::CursorShape;
pub use highlight::Highlight;
pub(crate) use palette::ColorSource;
pub use palette::{Palette, Rgb};
pub use row::{BackgroundSpan, Row, Segment, StyleFlags, StyleSpan, Underline};
pub(crate) use row::{Builder, cell_columns};
pub(crate) use viewport::Viewport;

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
