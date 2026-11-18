//! One visible line of the grid, ready to draw.

mod background_span;
mod row_builder;
mod segment;
mod style_flags;
mod style_span;
mod underline;

pub use background_span::BackgroundSpan;
pub(crate) use row_builder::{RowBuilder, cell_columns};
pub use segment::Segment;
pub use style_flags::StyleFlags;
pub use style_span::StyleSpan;
pub use underline::Underline;

/// The drawable content of a screen row: fills under the text, then the text.
#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
pub struct Row {
    /// Non-default backgrounds, left to right.
    pub backgrounds: Vec<BackgroundSpan>,
    /// Text pieces, left to right.
    pub segments: Vec<Segment>,
}
