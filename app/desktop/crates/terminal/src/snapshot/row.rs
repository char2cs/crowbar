mod background_span;
mod builder;
mod cell_colors;
mod segment;
mod style_flags;
mod style_span;
mod underline;

pub use background_span::BackgroundSpan;
pub(crate) use builder::{Builder, cell_columns};
pub use segment::Segment;
pub use style_flags::StyleFlags;
pub use style_span::StyleSpan;
pub use underline::Underline;

#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
pub struct Row {
    pub backgrounds: Vec<BackgroundSpan>,
    pub segments: Vec<Segment>,
}
