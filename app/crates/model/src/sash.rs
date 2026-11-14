use crate::axis::Axis;
use crate::branch::Branch;
use crate::rect::Rect;

/// The draggable divider of one split, as laid out in a given rectangle.
#[derive(Debug, Clone, PartialEq)]
pub struct Sash {
    /// Steps from the tab's root to the split this sash belongs to; pass it to
    /// [`Command::ResizeSplit`](crate::Command::ResizeSplit).
    pub path: Vec<Branch>,
    /// The axis the split arranges its children along; the sash is perpendicular to it.
    pub axis: Axis,
    /// The divider's rectangle.
    pub rect: Rect,
}
