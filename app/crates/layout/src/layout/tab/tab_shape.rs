use protocol::PaneId;

use crate::space_id::SpaceId;
use crate::tab_id::TabId;
use crate::tree::split_node::SplitNode;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct TabShape {
    pub(crate) id: TabId,
    pub(crate) title: String,
    pub(crate) space: SpaceId,
    pub(crate) tree: Option<SplitNode>,
    pub(crate) focused: Option<PaneId>,
}
