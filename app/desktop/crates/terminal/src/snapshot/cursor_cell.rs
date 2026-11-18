//! The cursor as the view draws it.

use crate::snapshot::cursor_shape::CursorShape;

/// Where the cursor is, its shape, and the glyph under it (redrawn in the background colour for a
/// block cursor).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CursorCell {
    /// Visible row.
    pub row: usize,
    /// Column.
    pub column: usize,
    /// Cells the glyph under the cursor spans (2 for a wide glyph).
    pub columns: usize,
    /// Shape.
    pub shape: CursorShape,
    /// The character in the cursor's cell.
    pub character: char,
}
