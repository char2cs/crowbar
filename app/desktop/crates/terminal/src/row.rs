//! One visible line of the grid, ready to draw.

use crate::background_span::BackgroundSpan;
use crate::segment::Segment;

/// The drawable content of a screen row: fills under the text, then the text.
#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
pub struct Row {
    /// Non-default backgrounds, left to right.
    pub backgrounds: Vec<BackgroundSpan>,
    /// Text pieces, left to right.
    pub segments: Vec<Segment>,
}
