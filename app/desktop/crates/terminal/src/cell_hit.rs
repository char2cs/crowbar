//! The cell under a pointer.

use crate::cell_side::CellSide;
use crate::drag_edge::DragEdge;
use crate::view_point::ViewPoint;

/// A pointer position resolved to a cell, the half of it, and whether it lies past the grid.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CellHit {
    /// The cell, clamped to the grid.
    pub point: ViewPoint,
    /// The half of the cell.
    pub side: CellSide,
    /// Vertical position relative to the grid.
    pub edge: DragEdge,
}
