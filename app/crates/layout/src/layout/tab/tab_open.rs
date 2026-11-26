use protocol::PaneId;

use crate::error::{Error, Result};
use crate::ide::pane_group::PaneGroup;
use crate::layout::tab::pane_instance::PaneInstance;
use crate::layout::tab::{MAX_PANES_PER_TAB, Tab};
use crate::tree::edge::Edge;
use crate::tree::placement::Placement;
use crate::tree::split_node::SplitNode;

impl Tab {
    pub(crate) fn check_open(
        &self,
        id: &PaneId,
        placement: Option<&Placement>,
    ) -> Result<()> {
        if self.panes.contains_key(id) {
            return Err(Error::DuplicatePane(id.clone()));
        }
        (self.panes.len() < MAX_PANES_PER_TAB)
            .then_some(())
            .ok_or(Error::TooManyPanes {
                tab: self.id,
                limit: MAX_PANES_PER_TAB,
            })?;
        if self.tree.is_none() {
            return Ok(());
        }
        let target = self.target_of(placement)?;
        self.require(&target)
    }

    pub(crate) fn open(
        &mut self,
        instance: PaneInstance,
        placement: Option<Placement>,
    ) -> Result<()> {
        let id = instance.pane_id.clone();
        self.check_open(&id, placement.as_ref())?;
        let joins = placement.is_none() && self.tree.is_some() && !instance.type_id.is_chat();
        if joins {
            let focused = self.target_of(None)?;
            return self.open_in_group(instance, (&focused, None));
        }
        self.insert_node(SplitNode::Leaf(PaneGroup::holding(&instance)), placement)?;
        self.focused = Some(id.clone());
        self.panes.insert(id, instance);
        Ok(())
    }

    pub(crate) fn insert_node(
        &mut self,
        fresh: SplitNode,
        placement: Option<Placement>,
    ) -> Result<()> {
        if self.tree.is_none() {
            self.tree = Some(fresh);
            return Ok(());
        }
        let placement = self.resolve(placement)?;
        let split = self
            .tree
            .as_mut()
            .is_some_and(|tree| tree.split_leaf(&placement.target, fresh, placement.edge));
        split
            .then_some(())
            .ok_or(Error::PaneNotFound(placement.target))
    }

    pub(crate) fn take_pane(
        &mut self,
        pane: &PaneId,
    ) -> Result<PaneInstance> {
        self.require(pane)?;
        let heir = self.heir_of(pane);
        let instance = self
            .panes
            .remove(pane)
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
        self.tree = self.tree.take().and_then(|tree| tree.without(pane));
        if self.focused.as_ref() == Some(pane) {
            self.focused = heir;
        }
        Ok(instance)
    }

    fn heir_of(
        &self,
        pane: &PaneId,
    ) -> Option<PaneId> {
        let tree = self.tree.as_ref()?;
        let mut group = tree.group_of(pane)?.clone();
        group.take(pane);
        group.shown().or_else(|| tree.neighbour_of(pane)).cloned()
    }

    pub(crate) fn target_of(
        &self,
        placement: Option<&Placement>,
    ) -> Result<PaneId> {
        match placement {
            Some(placement) => Ok(placement.target.clone()),
            None => self
                .focused
                .clone()
                .ok_or(Error::PlacementRequired(self.id)),
        }
    }

    fn resolve(
        &self,
        placement: Option<Placement>,
    ) -> Result<Placement> {
        match placement {
            Some(placement) => Ok(placement),
            None => Ok(Placement {
                target: self.target_of(None)?,
                edge: Edge::Right,
            }),
        }
    }

    pub(crate) fn require(
        &self,
        pane: &PaneId,
    ) -> Result<()> {
        self.panes
            .contains_key(pane)
            .then_some(())
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tab_id::TabId;
    use crate::test_support::tab_fixtures::{at, editor, id, order, pane, tab_with};

    fn group_of<'a>(
        tab: &'a Tab,
        name: &str,
    ) -> &'a PaneGroup {
        tab.group_of(&id(name)).unwrap()
    }

    #[test]
    fn chats_split_right_of_the_focus_each_in_a_group_with_the_editor_closed() {
        let tab = tab_with(&["a"]);
        assert_eq!((tab.len(), tab.is_empty()), (1, false), "size");
        let tab = tab_with(&["a", "b", "c"]);
        assert_eq!(order(&tab), ["a", "b", "c"], "each right of the last");
        assert_eq!(tab.focused(), Some(&id("c")), "new pane focused");
        let group = group_of(&tab, "a");
        assert_eq!(group.chat(), Some(&id("a")), "the chat");
        assert!(group.tabs().is_empty(), "no editor tab");
        assert!(!group.editor_open(), "editor closed");
        assert_eq!(tab.group_count(), 3, "one place per chat");
        tab.validate().unwrap();
    }

    #[test]
    fn other_panes_join_the_focused_group_unless_placed_at_an_edge() {
        let mut tab = tab_with(&["a", "b"]);
        tab.open(editor("t1"), None).unwrap();
        tab.open(editor("t2"), None).unwrap();
        let group = group_of(&tab, "b");
        assert_eq!(group.tabs(), [id("t1"), id("t2")], "in opening order");
        assert_eq!(group.active_tab(), Some(&id("t2")), "the newest is active");
        assert!(group.editor_open(), "editor shown");
        assert!(!group.chat_selected(), "tab selected");
        assert_eq!(tab.focused(), Some(&id("t2")), "focused");
        assert_eq!(tab.group_count(), 2, "no new place");
        tab.open(editor("t3"), Some(at("a", Edge::Right))).unwrap();
        assert_eq!(tab.group_count(), 3, "a new place");
        assert_eq!(group_of(&tab, "t3").chat(), None, "without a chat");
        tab.validate().unwrap();
    }

    #[test]
    fn open_refuses_without_changes() {
        let mut full = Tab::new(TabId(1), "t", crate::space_id::SpaceId::default());
        for index in 0..MAX_PANES_PER_TAB {
            full.open(pane(&format!("p{index}")), None).unwrap();
        }
        let mut unfocused = tab_with(&["a", "b"]);
        unfocused.focused = None;
        let limit = MAX_PANES_PER_TAB;
        let cases = [
            (tab_with(&["a"]), "a", None, Error::DuplicatePane(id("a"))),
            (
                tab_with(&["a"]),
                "b",
                Some("zz"),
                Error::PaneNotFound(id("zz")),
            ),
            (unfocused, "c", None, Error::PlacementRequired(TabId(1))),
            (
                full,
                "c",
                None,
                Error::TooManyPanes {
                    tab: TabId(1),
                    limit,
                },
            ),
        ];
        for (mut tab, new, target, error) in cases {
            let before = tab.clone();
            let placement = target.map(|target| at(target, Edge::Left));
            assert_eq!(tab.open(pane(new), placement), Err(error.clone()));
            assert_eq!(tab, before, "{error}: unchanged");
            tab.validate().unwrap();
        }
    }

    #[test]
    fn closing_hands_focus_to_the_pane_taking_the_space_or_keeps_it() {
        let mut tab = tab_with(&["a", "b", "c"]);
        tab.focus(&id("b")).unwrap();
        let closed = tab.take_pane(&id("b")).unwrap();
        assert_eq!(closed.pane_id, id("b"), "returned instance");
        assert_eq!(tab.focused(), Some(&id("c")), "heir focused");
        assert_eq!(order(&tab), ["a", "c"], "order");
        tab.focus(&id("a")).unwrap();
        tab.take_pane(&id("c")).unwrap();
        assert_eq!(tab.focused(), Some(&id("a")), "focus kept");
        tab.take_pane(&id("a")).unwrap();
        assert!(tab.tree().is_none(), "empty tree");
        assert!(tab.focused().is_none(), "no focus");
        let again = tab.take_pane(&id("a"));
        assert_eq!(again, Err(Error::PaneNotFound(id("a"))), "again");
        tab.validate().unwrap();
    }

    #[test]
    fn closing_inside_a_group_keeps_the_rest_of_it() {
        let mut tab = tab_with(&["a"]);
        tab.open(editor("t1"), None).unwrap();
        let mut chatless = tab.clone();
        tab.take_pane(&id("t1")).unwrap();
        let group = group_of(&tab, "a");
        assert!(group.tabs().is_empty(), "no tabs");
        assert!(!group.editor_open(), "editor closed");
        assert_eq!(tab.focused(), Some(&id("a")), "the chat");
        chatless.take_pane(&id("a")).unwrap();
        assert_eq!(group_of(&chatless, "t1").chat(), None, "chatless");
        assert_eq!(chatless.group_count(), 1, "same place");
        assert_eq!(chatless.focused(), Some(&id("t1")), "focus on the tab");
        tab.validate().unwrap();
        chatless.validate().unwrap();
    }
}
