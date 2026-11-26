use protocol::PaneId;

use crate::ide::pane_group::PaneGroup;
use crate::layout::tab::Tab;
use crate::tree::geometry::rect::Rect;
use crate::tree::split_node::SplitNode;

impl Tab {
    /// The rectangle of every group when the tab is laid out in `bounds`, left to right and top
    /// to bottom. [`Tab::layout`] lists only the pane each of these shows.
    #[must_use]
    pub fn leaves(
        &self,
        bounds: Rect,
    ) -> Vec<(Rect, &PaneGroup)> {
        self.tree
            .as_ref()
            .map_or_else(Vec::new, |tree| tree.place_leaves(bounds))
    }

    /// The group that holds `pane`.
    #[must_use]
    pub fn group_of(
        &self,
        pane: &PaneId,
    ) -> Option<&PaneGroup> {
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
            .is_some_and(|group| group.shown() == Some(pane))
    }
}

#[cfg(test)]
mod tests {
    use protocol::{PaneState, PaneTypeId, Params};

    use super::*;
    use crate::layout::tab::pane_instance::PaneInstance;
    use crate::tab_id::TabId;
    use crate::tree::edge::Edge;
    use crate::tree::placement::Placement;

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
        let mut tab = Tab::new(TabId(1), "t", crate::space_id::SpaceId::default());
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
            tab.group_of(&id("a")).map(PaneGroup::len),
            Some(2),
            "strip of two"
        );
        assert!(tab.group_of(&id("q")).is_none(), "unknown");
        assert_eq!(
            Tab::new(TabId(2), "e", crate::space_id::SpaceId::default()).group_count(),
            0,
            "empty tab"
        );
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
    fn leaves_list_every_group_with_its_rectangle() {
        let tab = two_groups();
        let bounds = Rect {
            x: 0.0,
            y: 0.0,
            width: 1000.0,
            height: 500.0,
        };
        let leaves = tab.leaves(bounds);
        let (rect, first) = leaves.first().unwrap();
        assert_eq!(first.tabs(), [id("a"), id("b")], "members");
        assert_eq!(first.active_tab(), Some(&id("b")), "shown");
        assert!(rect.width < bounds.width, "beside the other group");
        assert_eq!(leaves.len(), 2, "one per group");
    }
}
