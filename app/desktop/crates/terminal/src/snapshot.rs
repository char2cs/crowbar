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

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Snapshot {
    pub rows: Vec<Row>,
    pub cursor: Option<CursorCell>,
    pub cursor_blinks: bool,
    pub selection: Vec<Highlight>,
    pub matches: Vec<Highlight>,
    pub current_match: Vec<Highlight>,
    pub display_offset: usize,
    pub history_size: usize,
}
