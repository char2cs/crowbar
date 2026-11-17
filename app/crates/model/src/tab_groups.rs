use protocol::PaneId;

use crate::error::{Error, Result};
use crate::group::Group;
use crate::group_slot::GroupSlot;
use crate::pane_instance::PaneInstance;
use crate::rect::Rect;
use crate::split_node::SplitNode;
use crate::tab::{MAX_PANES_PER_TAB, Tab};

impl Tab {
    /// The rectangle of every group, with the panes stacked in it, when the tab is laid out in
    /// `bounds`. [`Tab::layout`] lists only the pane each of these shows.
    #[must_use]
    pub fn groups(
        &self,
        bounds: Rect,
    ) -> Vec<GroupSlot> {
        self.tree
            .as_ref()
            .map_or_else(Vec::new, |tree| tree.place_groups(bounds))
    }

    /// The group that holds `pane`.
    #[must_use]
    pub fn group_of(
        &self,
        pane: &PaneId,
    ) -> Option<&Group> {
        self.tree.as_ref().and_then(|tree| tree.group_of(pane))
    }

    /// Number of groups, which is the number of places in the layout.
    #[must_use]
    pub fn group_count(&self) -> usize {
        self.tree.as_ref().map_or(0, SplitNode::leaf_count)
    }

    pub(crate) fn show(
        &mut self,
        pane: &PaneId,
    ) {
        if let Some(group) = self.tree.as_mut().and_then(|tree| tree.group_of_mut(pane)) {
            group.activate(pane);
        }
    }

    pub(crate) fn is_shown(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.group_of(pane)
            .is_some_and(|group| group.active() == pane)
    }

    pub(crate) fn check_open_in_group(
        &self,
        id: &PaneId,
        onto: (&PaneId, Option<usize>),
    ) -> Result<()> {
        if self.panes.contains_key(id) {
            return Err(Error::DuplicatePane(id.clone()));
        }
        (self.panes.len() < MAX_PANES_PER_TAB)
            .then_some(())
            .ok_or(Error::TooManyPanes {
                tab: self.id(),
                limit: MAX_PANES_PER_TAB,
            })?;
        let group = self
            .group_of(onto.0)
            .ok_or_else(|| Error::PaneNotFound(onto.0.clone()))?;
        Group::check_index(onto.1, group.len()).map(drop)
    }

    pub(crate) fn open_in_group(
        &mut self,
        instance: PaneInstance,
        onto: (&PaneId, Option<usize>),
    ) -> Result<()> {
        let id = instance.pane_id.clone();
        self.check_open_in_group(&id, onto)?;
        let group = self
            .tree
            .as_mut()
            .and_then(|tree| tree.group_of_mut(onto.0))
            .ok_or_else(|| Error::PaneNotFound(onto.0.clone()))?;
        group.insert(id.clone(), onto.1)?;
        self.focused = Some(id.clone());
        self.panes.insert(id, instance);
        Ok(())
    }

    pub(crate) fn join_group(
        &mut self,
        pane: &PaneId,
        onto: (&PaneId, Option<usize>),
    ) -> Result<()> {
        self.require(pane)?;
        self.require(onto.0)?;
        let same = self
            .group_of(pane)
            .is_some_and(|group| group.contains(onto.0));
        if same {
            return self.reorder(pane, onto.1);
        }
        let destination = self
            .group_of(onto.0)
            .ok_or_else(|| Error::PaneNotFound(onto.0.clone()))?;
        Group::check_index(onto.1, destination.len())?;
        let mut rest = self
            .tree
            .clone()
            .and_then(|tree| tree.without(pane))
            .ok_or_else(|| Error::PaneNotFound(onto.0.clone()))?;
        rest.group_of_mut(onto.0)
            .ok_or_else(|| Error::PaneNotFound(onto.0.clone()))?
            .insert(pane.clone(), onto.1)?;
        self.tree = Some(rest);
        self.focused = Some(pane.clone());
        Ok(())
    }

    fn reorder(
        &mut self,
        pane: &PaneId,
        index: Option<usize>,
    ) -> Result<()> {
        let group = self
            .tree
            .as_mut()
            .and_then(|tree| tree.group_of_mut(pane))
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
        group.reorder(pane, index)?;
        self.focused = Some(pane.clone());
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use protocol::{PaneState, PaneTypeId, Params};

    use super::*;
    use crate::edge::Edge;
    use crate::placement::Placement;
    use crate::tab_id::TabId;

    fn id(name: &str) -> PaneId {
        PaneId::new(name).unwrap()
    }

    fn pane(name: &str) -> PaneInstance {
        PaneInstance {
            pane_id: id(name),
            type_id: PaneTypeId::new("core.terminal").unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        }
    }

    fn two_groups() -> Tab {
        let mut tab = Tab::new(TabId(1), "t");
        tab.open(pane("a"), None).unwrap();
        tab.open_in_group(pane("b"), (&id("a"), None)).unwrap();
        let beside = Placement {
            target: id("a"),
            edge: Edge::Right,
        };
        tab.open(pane("c"), Some(beside)).unwrap();
        tab
    }

    #[test]
    fn a_tab_counts_groups_and_finds_a_pane_s_group() {
        let tab = two_groups();
        assert_eq!(tab.group_count(), 2, "groups");
        assert_eq!(tab.len(), 3, "panes");
        assert_eq!(
            tab.group_of(&id("a")).map(Group::len),
            Some(2),
            "strip of two"
        );
        assert!(tab.group_of(&id("q")).is_none(), "unknown");
        assert_eq!(Tab::new(TabId(2), "e").group_count(), 0, "empty tab");
    }

    const CASES: [(&str, &str, bool); 3] = [
        ("b", "the newest is shown", true),
        ("a", "its sibling is hidden", false),
        ("c", "a lone pane is shown", true),
    ];

    #[test]
    fn only_the_shown_pane_of_a_group_is_shown() {
        let mut tab = two_groups();
        for (pane, name, expected) in CASES {
            assert_eq!(tab.is_shown(&id(pane)), expected, "{name}");
        }
        tab.show(&id("a"));
        assert!(tab.is_shown(&id("a")), "now shown");
    }

    #[test]
    fn group_slots_list_members_in_strip_order_with_the_shown_one() {
        let tab = two_groups();
        let bounds = Rect {
            x: 0.0,
            y: 0.0,
            width: 1000.0,
            height: 500.0,
        };
        let slots = tab.groups(bounds);
        let first = slots.first().unwrap();
        assert_eq!(first.panes, [id("a"), id("b")], "members");
        assert_eq!(first.active, id("b"), "shown");
        assert_eq!(slots.len(), 2, "one slot per group");
    }

    #[test]
    fn joining_a_group_of_the_same_tab_refuses_a_bad_index_without_changes() {
        let mut tab = two_groups();
        let before = tab.clone();
        let refused = tab.join_group(&id("c"), (&id("a"), Some(5)));
        assert_eq!(
            refused,
            Err(Error::IndexOutOfRange { index: 5, len: 2 }),
            "index"
        );
        assert_eq!(tab, before, "unchanged");
    }
}
