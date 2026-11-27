use protocol::PaneId;

use crate::tree::geometry::rect::Rect;

#[derive(Debug, Clone, PartialEq)]
pub struct PaneSlot {
    pub pane: PaneId,
    pub rect: Rect,
}
