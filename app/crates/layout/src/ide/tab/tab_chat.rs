use protocol::PaneId;

use crate::error::{Error, Result};
use crate::layout::tab::Tab;
use crate::tree::split_node::SplitNode;

impl Tab {
    pub(crate) fn is_chat(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.panes
            .get(pane)
            .is_some_and(|instance| instance.type_id.is_chat())
    }

    pub(crate) fn lifted(
        &self,
        pane: &PaneId,
    ) -> (Option<SplitNode>, SplitNode) {
        let Some(tree) = self.tree.clone() else {
            return (None, SplitNode::leaf(pane.clone()));
        };
        let group = tree
            .group_of(pane)
            .filter(|group| group.chat() == Some(pane));
        match group.cloned() {
            Some(group) => (tree.without_group_of(pane), SplitNode::Leaf(group)),
            None => (tree.without(pane), SplitNode::leaf(pane.clone())),
        }
    }

    pub(crate) fn check_swappable(
        &self,
        a: &PaneId,
        b: &PaneId,
    ) -> Result<()> {
        match (self.is_chat(a), self.is_chat(b)) {
            (true, false) => Err(Error::ChatIsNotATab(a.clone())),
            (false, true) => Err(Error::ChatIsNotATab(b.clone())),
            _ => Ok(()),
        }
    }
}

#[cfg(test)]
mod tests {
    use crate::test_support::tab_fixtures::{editor, id, order, tab_with};

    #[test]
    fn swapping_two_chats_exchanges_their_whole_groups() {
        let mut tab = tab_with(&["a", "b"]);
        tab.open(editor("t1"), None).unwrap();
        tab.swap(&id("a"), &id("b")).unwrap();
        assert_eq!(order(&tab), ["b", "t1", "a"], "b keeps its tab");
        tab.validate().unwrap();
    }
}
