use crate::tree::axis::Axis;
use crate::tree::branch::Branch;
use crate::tree::geometry::rect::Rect;

#[derive(Debug, Clone, PartialEq)]
pub struct Sash {
    pub path: Vec<Branch>,
    pub axis: Axis,
    pub rect: Rect,
}
