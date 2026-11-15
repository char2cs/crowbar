//! Cursor shapes.

/// The shape the cursor is drawn with.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum CursorShape {
    /// A filled cell.
    #[default]
    Block,
    /// A line under the cell.
    Underline,
    /// A vertical bar at the cell's left edge.
    Bar,
}
