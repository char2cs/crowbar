use std::collections::{BTreeMap, BTreeSet};

use protocol::PaneId;

use crate::space_id::SpaceId;
use crate::split_node::SplitNode;
use crate::tab_id::TabId;
use crate::tab_shape::TabShape;

#[derive(Debug, Clone, Default, PartialEq)]
pub(crate) struct Arrangement {
    pub(crate) tabs: Vec<TabShape>,
    pub(crate) shown: BTreeMap<SpaceId, TabId>,
}

impl Arrangement {
    pub(crate) fn spaces_changed_in(
        &self,
        after: &Self,
    ) -> BTreeSet<SpaceId> {
        let spaces = self.tabs.iter().chain(&after.tabs).map(|tab| &tab.space);
        let spaces: BTreeSet<&SpaceId> = spaces
            .chain(self.shown.keys())
            .chain(after.shown.keys())
            .collect();
        spaces
            .into_iter()
            .filter(|space| self.of(space) != after.of(space))
            .cloned()
            .collect()
    }

    pub(crate) fn adopt(
        &mut self,
        (id, space): (TabId, &SpaceId),
    ) {
        let unowned = |tab: &&mut TabShape| tab.id == id && tab.space.is_unowned();
        let Some(tab) = self.tabs.iter_mut().find(unowned) else {
            return;
        };
        tab.space = space.clone();
        crate::tab_commands::adopt_shown(&mut self.shown, (id, space.clone()));
    }

    fn of(
        &self,
        space: &SpaceId,
    ) -> (Vec<&TabShape>, Option<&TabId>) {
        let tabs = self.tabs.iter().filter(|tab| &tab.space == space).collect();
        (tabs, self.shown.get(space))
    }

    pub(crate) fn pane_ids(&self) -> Vec<PaneId> {
        self.tabs
            .iter()
            .filter_map(|tab| tab.tree.as_ref())
            .flat_map(SplitNode::pane_ids)
            .collect()
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
            space: SpaceId::default(),
            tree: Some(SplitNode::leaf(id(pane))),
            focused: Some(id(pane)),
        };
        let empty = TabShape {
            id: TabId(3),
            title: String::new(),
            space: SpaceId::default(),
            tree: None,
            focused: None,
        };
        let arrangement = Arrangement {
            tabs: vec![shape(1, "a"), empty, shape(2, "b")],
            shown: BTreeMap::new(),
        };
        assert_eq!(
            arrangement.pane_ids(),
            [id("a"), id("b")],
            "leaves in tab order"
        );
    }
}
