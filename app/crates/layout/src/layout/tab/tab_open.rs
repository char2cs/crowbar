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
    fn a_chat_opens_in_a_group_of_its_own_with_the_editor_closed() {
        let tab = tab_with(&["a", "b"]);
        let group = group_of(&tab, "a");
        assert_eq!(group.chat(), Some(&id("a")), "the chat");
        assert!(group.tabs().is_empty(), "no editor tab");
        assert!(!group.editor_open(), "editor closed");
        assert_eq!(tab.group_count(), 2, "one place per chat");
    }

    #[test]
    fn any_other_pane_joins_the_focused_group_as_its_active_editor_tab() {
        let mut tab = tab_with(&["a", "b"]);
        tab.open(editor("t1"), None).unwrap();
        tab.open(editor("t2"), None).unwrap();
        let group = group_of(&tab, "b");
        assert_eq!(group.tabs(), [id("t1"), id("t2")], "tabs in opening order");
        assert_eq!(group.active_tab(), Some(&id("t2")), "the newest is active");
        assert!(
            group.editor_open() && !group.chat_selected(),
            "editor shown"
        );
        assert_eq!(tab.group_count(), 2, "no new place");
        assert_eq!(tab.focused(), Some(&id("t2")), "focused");
        tab.validate().unwrap();
    }

    #[test]
    fn an_editor_pane_placed_at_an_edge_opens_a_group_of_its_own() {
        let mut tab = tab_with(&["a"]);
        tab.open(editor("t1"), Some(at("a", Edge::Right))).unwrap();
        assert_eq!(tab.group_count(), 2, "a new place");
        assert_eq!(group_of(&tab, "t1").chat(), None, "without a chat");
        tab.validate().unwrap();
    }

    #[test]
    fn closing_the_last_editor_tab_closes_the_editor_and_focuses_the_chat() {
        let mut tab = tab_with(&["a"]);
        tab.open(editor("t1"), None).unwrap();
        tab.take_pane(&id("t1")).unwrap();
        let group = group_of(&tab, "a");
        assert!(
            group.tabs().is_empty() && !group.editor_open(),
            "editor closed"
        );
        assert_eq!(tab.focused(), Some(&id("a")), "the chat");
        tab.validate().unwrap();
    }

    #[test]
    fn closing_the_chat_leaves_its_editor_tabs_in_place() {
        let mut tab = tab_with(&["a"]);
        tab.open(editor("t1"), None).unwrap();
        tab.take_pane(&id("a")).unwrap();
        assert_eq!(group_of(&tab, "t1").chat(), None, "chatless");
        assert_eq!(tab.group_count(), 1, "same place");
        assert_eq!(tab.focused(), Some(&id("t1")), "focus on the tab");
        tab.validate().unwrap();
    }

    #[test]
    fn first_pane_becomes_root_and_focus() {
        let tab = tab_with(&["a"]);
        assert_eq!(order(&tab), ["a"], "order");
        assert_eq!(tab.focused(), Some(&id("a")), "focus");
        assert_eq!((tab.len(), tab.is_empty()), (1, false), "size");
        tab.validate().unwrap();
    }

    #[test]
    fn default_placement_splits_the_focused_pane_to_the_right() {
        let tab = tab_with(&["a", "b", "c"]);
        assert_eq!(order(&tab), ["a", "b", "c"], "each right of the previous");
        assert_eq!(tab.focused(), Some(&id("c")), "new pane focused");
    }

    #[test]
    fn open_validates_before_changing_anything() {
        let cases = [
            ("duplicate", pane("a"), None, Error::DuplicatePane(id("a"))),
            (
                "unknown target",
                pane("b"),
                Some(at("zz", Edge::Left)),
                Error::PaneNotFound(id("zz")),
            ),
        ];
        for (name, instance, placement, error) in cases {
            let mut tab = tab_with(&["a"]);
            let before = tab.clone();
            assert_eq!(tab.open(instance, placement), Err(error), "{name}");
            assert_eq!(tab, before, "{name}: unchanged after refusal");
        }
    }

    #[test]
    fn open_without_focus_needs_a_placement() {
        let mut tab = tab_with(&["a", "b"]);
        tab.focused = None;
        assert_eq!(
            tab.open(pane("c"), None),
            Err(Error::PlacementRequired(TabId(1))),
            "no focus"
        );
    }

    #[test]
    fn a_tab_holds_a_bounded_number_of_panes() {
        let mut tab = Tab::new(TabId(1), "t", crate::space_id::SpaceId::default());
        for index in 0..MAX_PANES_PER_TAB {
            tab.open(pane(&format!("p{index}")), None).unwrap();
        }
        let before = tab.clone();
        assert_eq!(
            tab.open(pane("one-more"), None),
            Err(Error::TooManyPanes {
                tab: TabId(1),
                limit: MAX_PANES_PER_TAB
            }),
            "full"
        );
        assert_eq!(tab, before, "unchanged");
        tab.validate().unwrap();
    }

    #[test]
    fn closing_the_right_pane_of_a_nested_row_focuses_the_pane_beside_it() {
        let mut tab = Tab::new(TabId(1), "t", crate::space_id::SpaceId::default());
        tab.open(pane("a"), None).unwrap();
        tab.open(pane("x"), Some(at("a", Edge::Right))).unwrap();
        tab.open(pane("b"), Some(at("a", Edge::Right))).unwrap();
        tab.focus(&id("x")).unwrap();
        assert_eq!(order(&tab), ["a", "b", "x"], "[[a|b]|x]");
        tab.take_pane(&id("x")).unwrap();
        assert_eq!(
            tab.focused(),
            Some(&id("b")),
            "b is adjacent, not the far left a"
        );
        tab.validate().unwrap();
    }

    #[test]
    fn closing_a_pane_hands_focus_to_the_neighbour_taking_its_space() {
        let mut tab = tab_with(&["a", "b", "c"]);
        tab.focus(&id("b")).unwrap();
        let closed = tab.take_pane(&id("b")).unwrap();
        assert_eq!(closed.pane_id, id("b"), "returned instance");
        assert_eq!(tab.focused(), Some(&id("c")), "heir focused");
        assert_eq!(order(&tab), ["a", "c"], "order");
        tab.validate().unwrap();
    }

    #[test]
    fn closing_an_unfocused_pane_keeps_focus() {
        let mut tab = tab_with(&["a", "b"]);
        tab.focus(&id("a")).unwrap();
        tab.take_pane(&id("b")).unwrap();
        assert_eq!(tab.focused(), Some(&id("a")), "focus kept");
    }

    #[test]
    fn removing_the_last_pane_empties_the_tab() {
        let mut tab = tab_with(&["a"]);
        tab.take_pane(&id("a")).unwrap();
        assert!(tab.tree().is_none() && tab.focused().is_none(), "empty");
        tab.validate().unwrap();
        assert_eq!(
            tab.take_pane(&id("a")),
            Err(Error::PaneNotFound(id("a"))),
            "again"
        );
    }
}
