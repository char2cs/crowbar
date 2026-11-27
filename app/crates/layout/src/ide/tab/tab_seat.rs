use protocol::PaneId;

use crate::error::{Error, Result};
use crate::ide::pane_group::PaneGroup;
use crate::layout::tab::pane_instance::PaneInstance;
use crate::layout::tab::{MAX_PANES_PER_TAB, Tab};

impl Tab {
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
        PaneGroup::check_index(onto.1, group.len()).map(drop)
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
        PaneGroup::check_index(onto.1, destination.len())?;
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
