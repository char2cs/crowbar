use protocol::PaneId;

use crate::tree::edge::Edge;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Placement {
    pub target: PaneId,
    pub edge: Edge,
}
