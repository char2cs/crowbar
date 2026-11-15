//! Whether a drag has left the grid vertically.

/// Where a dragging pointer is relative to the grid; outside means the view should keep scrolling.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DragEdge {
    /// Over the grid.
    Inside,
    /// Above the first row.
    Above,
    /// Below the last row.
    Below,
}
