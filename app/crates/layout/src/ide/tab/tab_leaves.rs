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
    use super::*;
    use crate::tab_id::TabId;
    use crate::test_support::tab_fixtures::{at, editor, id, screen};
    use crate::tree::edge::Edge;

    #[test]
    fn only_the_shown_pane_of_a_group_is_shown() {
        let mut tab = Tab::new(TabId(1), "t", crate::space_id::SpaceId::default());
        tab.open(editor("a"), None).unwrap();
        tab.open_in_group(editor("b"), (&id("a"), None)).unwrap();
        tab.open(editor("c"), Some(at("a", Edge::Right))).unwrap();
        let shown = ["a", "b", "c"].map(|pane| tab.is_shown(&id(pane)));
        assert_eq!(shown, [false, true, true], "the newest and a lone pane");
        tab.show(&id("a"));
        assert!(tab.is_shown(&id("a")), "now shown");
        let empty = Tab::new(TabId(2), "e", crate::space_id::SpaceId::default());
        assert_eq!(empty.group_count(), 0, "empty tab");
        assert!(empty.leaves(screen(1.0, 1.0)).is_empty(), "no leaves");
    }
}
