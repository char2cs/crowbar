use protocol::PaneId;

use crate::rect::Rect;

/// The screen rectangle of one group and the panes stacked in it.
#[derive(Debug, Clone, PartialEq)]
pub struct GroupSlot {
    /// Where the group is placed.
    pub rect: Rect,
    /// The members in strip order.
    pub panes: Vec<PaneId>,
    /// The member that is shown.
    pub active: PaneId,
}
