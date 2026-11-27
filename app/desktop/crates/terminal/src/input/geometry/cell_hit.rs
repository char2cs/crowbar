use crate::input::geometry::cell_side::CellSide;
use crate::input::geometry::drag_edge::DragEdge;
use crate::input::geometry::view_point::ViewPoint;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CellHit {
    pub point: ViewPoint,
    pub side: CellSide,
    pub edge: DragEdge,
}
