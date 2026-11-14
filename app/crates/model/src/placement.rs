use protocol::PaneId;

use crate::edge::Edge;

/// Where a pane lands: on one side of an existing pane of the same tab.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Placement {
    /// The pane the new one is placed next to.
    pub target: PaneId,
    /// The side of the target.
    pub edge: Edge,
}
