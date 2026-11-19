use protocol::PaneId;

use crate::edge::Edge;
use crate::error::{Error, Result};
use crate::pane_instance::PaneInstance;
use crate::placement::Placement;
use crate::split_node::SplitNode;
use crate::tab::{MAX_PANES_PER_TAB, Tab};

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
        self.insert_leaf(&id, placement)?;
        self.focused = Some(id.clone());
        self.panes.insert(id, instance);
        Ok(())
    }

    fn insert_leaf(
        &mut self,
        id: &PaneId,
        placement: Option<Placement>,
    ) -> Result<()> {
        if self.tree.is_none() {
            self.tree = Some(SplitNode::leaf(id.clone()));
            return Ok(());
        }
        let placement = self.resolve(placement)?;
        let split = self
            .tree
            .as_mut()
            .is_some_and(|tree| tree.split_leaf(&placement.target, id, placement.edge));
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

    fn target_of(
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
    use crate::test_support::tab_fixtures::{at, id, order, pane, tab_with};

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
        let mut tab = Tab::new(TabId(1), "t");
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
        let mut tab = Tab::new(TabId(1), "t");
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
