use layout::Rect;
use protocol::PaneId;

use crate::geometry::corners::Corners;

/// The transparent rectangle of a pane where the pane host places its web view.
#[derive(Debug, Clone, PartialEq)]
pub struct PaneHole {
    /// The pane.
    pub pane: PaneId,
    /// Where its content shows, in the same coordinates as the layout's content area.
    pub rect: Rect,
    /// The corners the content rounds, where its rectangle meets its pane's rounded frame.
    pub radius: Corners,
}
