use protocol::PaneId;

use crate::split_node::SplitNode;
use crate::tab_id::TabId;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct TabShape {
    pub(crate) id: TabId,
    pub(crate) title: String,
    pub(crate) tree: Option<SplitNode>,
    pub(crate) focused: Option<PaneId>,
}
