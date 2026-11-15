use protocol::PaneId;

use crate::tab_id::TabId;
use crate::tab_shape::TabShape;

#[derive(Debug, Clone, Default, PartialEq)]
pub(crate) struct Arrangement {
    pub(crate) tabs: Vec<TabShape>,
    pub(crate) active: Option<TabId>,
}

impl Arrangement {
    pub(crate) fn pane_ids(&self) -> Vec<PaneId> {
        let mut ids = Vec::new();
        for tree in self.tabs.iter().filter_map(|tab| tab.tree.as_ref()) {
            tree.collect_panes(&mut ids);
        }
        ids
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::split_node::SplitNode;

    fn id(name: &str) -> PaneId {
        PaneId::new(name).unwrap()
    }

    #[test]
    fn pane_ids_lists_the_leaves_of_every_tab() {
        let shape = |tab: u64, pane: &str| TabShape {
            id: TabId(tab),
            title: String::new(),
            tree: Some(SplitNode::leaf(id(pane))),
            focused: Some(id(pane)),
        };
        let empty = TabShape {
            id: TabId(3),
            title: String::new(),
            tree: None,
            focused: None,
        };
        let arrangement = Arrangement {
            tabs: vec![shape(1, "a"), empty, shape(2, "b")],
            active: Some(TabId(1)),
        };
        assert_eq!(
            arrangement.pane_ids(),
            [id("a"), id("b")],
            "leaves in tab order"
        );
    }
}
