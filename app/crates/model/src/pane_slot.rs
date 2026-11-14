use protocol::PaneId;

use crate::rect::Rect;

/// The screen rectangle a pane occupies in the current layout.
#[derive(Debug, Clone, PartialEq)]
pub struct PaneSlot {
    /// The pane that fills the slot.
    pub pane: PaneId,
    /// Where it is placed.
    pub rect: Rect,
}
