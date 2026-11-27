use layout::Rect;
use protocol::PaneId;

use crate::geometry::corners::Corners;

#[derive(Debug, Clone, PartialEq)]
pub struct PaneHole {
    pub pane: PaneId,
    pub rect: Rect,
    pub radius: Corners,
}
