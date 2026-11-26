use protocol::PaneId;

use crate::error::{Error, Result};
use crate::ide::pane_group::PaneGroup;
use crate::layout::tab::pane_instance::PaneInstance;
use crate::layout::tab::{MAX_PANES_PER_TAB, Tab};
use crate::tree::placement::Placement;
use crate::tree::split_node::SplitNode;

pub(crate) type MovedGroup = (PaneGroup, Vec<PaneInstance>);

impl Tab {
    pub(crate) fn chat_count(&self) -> usize {
        self.panes
            .values()
            .filter(|instance| instance.type_id.is_chat())
            .count()
    }

    pub(crate) fn group_members(
        &self,
        chat: &PaneId,
    ) -> Result<Vec<PaneId>> {
        self.require(chat)?;
        if !self.is_chat(chat) {
            return Err(Error::NotAChat(chat.clone()));
        }
        Ok(self
            .group_of(chat)
            .map_or_else(Vec::new, PaneGroup::members))
    }

    pub(crate) fn take_group(
        &mut self,
        chat: &PaneId,
    ) -> Result<MovedGroup> {
        self.group_members(chat)?;
        let (rest, lifted) = self.lifted(chat);
        let SplitNode::Leaf(group) = lifted else {
            return Err(Error::NotAChat(chat.clone()));
        };
        let heir = self
            .tree
            .as_ref()
            .and_then(|tree| tree.neighbour_of(chat))
            .or_else(|| rest.as_ref().and_then(SplitNode::first_pane))
            .cloned();
        let instances = group
            .members()
            .iter()
            .filter_map(|member| self.panes.remove(member))
            .collect();
        if self
            .focused
            .as_ref()
            .is_some_and(|focused| group.contains(focused))
        {
            self.focused = heir;
        }
        self.tree = rest;
        Ok((group, instances))
    }

    pub(crate) fn check_receive(
        &self,
        members: &[PaneId],
        placement: Option<&Placement>,
    ) -> Result<()> {
        if let Some(present) = members
            .iter()
            .find(|member| self.panes.contains_key(*member))
        {
            return Err(Error::DuplicatePane(present.clone()));
        }
        let full = Error::TooManyPanes {
            tab: self.id,
            limit: MAX_PANES_PER_TAB,
        };
        if self.panes.len() + members.len() > MAX_PANES_PER_TAB {
            return Err(full);
        }
        if self.tree.is_none() {
            return Ok(());
        }
        let target = self.target_of(placement)?;
        self.require(&target)
    }

    pub(crate) fn receive(
        &mut self,
        moved: MovedGroup,
        placement: Option<Placement>,
    ) -> Result<()> {
        let (group, instances) = moved;
        let shown = group.shown().cloned();
        self.insert_node(SplitNode::Leaf(group), placement)?;
        self.focused = shown;
        let entries = instances
            .into_iter()
            .map(|instance| (instance.pane_id.clone(), instance));
        self.panes.extend(entries);
        Ok(())
    }

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
    fn a_chat_group_lifts_out_whole_and_lands_whole_with_what_it_showed() {
        let mut from = tab_with(&["a", "b"]);
        from.focus(&id("a")).unwrap();
        from.open(editor("t1"), None).unwrap();
        let moved = from.take_group(&id("a")).unwrap();
        assert_eq!(
            (order(&from), from.focused()),
            (vec!["b".to_owned()], Some(&id("b"))),
            "the rest keeps a focus"
        );
        let mut to = tab_with(&["c"]);
        to.receive(moved, None).unwrap();
        assert_eq!(order(&to), ["c", "a", "t1"], "beside the focused pane");
        assert_eq!(to.focused(), Some(&id("t1")), "the tab it showed");
        from.validate().unwrap();
        to.validate().unwrap();
    }

    #[test]
    fn swapping_two_chats_exchanges_their_whole_groups() {
        let mut tab = tab_with(&["a", "b"]);
        tab.open(editor("t1"), None).unwrap();
        tab.swap(&id("a"), &id("b")).unwrap();
        assert_eq!(order(&tab), ["b", "t1", "a"], "b keeps its tab");
        tab.validate().unwrap();
    }
}
