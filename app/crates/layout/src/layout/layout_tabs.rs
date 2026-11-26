use std::collections::BTreeMap;

use protocol::PaneId;

use crate::error::{Error, Result};
use crate::layout::Layout;
use crate::layout::arrangement::Arrangement;
use crate::layout::tab::Tab;
use crate::layout::tab::pane_instance::PaneInstance;
use crate::layout::undo_step::UndoStep;
use crate::tab_id::TabId;

impl Layout {
    pub(crate) fn arrangement(&self) -> Arrangement {
        Arrangement {
            tabs: self.tabs.iter().map(Tab::shape).collect(),
            shown: self.shown.clone(),
        }
    }

    pub(crate) fn swap_arrangement(
        &mut self,
        step: UndoStep,
    ) -> std::result::Result<UndoStep, UndoStep> {
        let restorable = step
            .arrangement
            .pane_ids()
            .iter()
            .all(|id| self.pane(id).is_some() || step.parked.contains_key(id));
        if !restorable {
            return Err(step);
        }
        let before = self.arrangement();
        let mut pool: BTreeMap<PaneId, PaneInstance> = step.parked;
        for tab in &mut self.tabs {
            pool.extend(tab.take_instances());
        }
        let Arrangement { tabs, shown } = step.arrangement;
        let changed = step.spaces;
        self.tabs = tabs
            .into_iter()
            .map(|shape| Tab::from_shape(shape, &mut pool))
            .collect();
        self.shown.retain(|space, _| !changed.contains(space));
        self.shown.extend(
            shown
                .into_iter()
                .filter(|(space, _)| changed.contains(space)),
        );
        Ok(UndoStep {
            arrangement: before,
            parked: pool,
            spaces: changed,
        })
    }

    pub(crate) fn show(
        &mut self,
        id: TabId,
    ) {
        if let Some(space) = self.tab(id).map(|tab| tab.space.clone()) {
            self.shown.insert(space, id);
        }
    }

    pub(crate) fn show_at(
        &mut self,
        index: usize,
    ) {
        if let Some(id) = self.tabs.get(index).map(Tab::id) {
            self.show(id);
        }
    }

    pub(crate) fn index_of(
        &self,
        id: TabId,
    ) -> Result<usize> {
        self.tabs
            .iter()
            .position(|tab| tab.id() == id)
            .ok_or(Error::TabNotFound(id))
    }

    pub(crate) fn index_of_pane(
        &self,
        pane: &PaneId,
    ) -> Result<usize> {
        self.tabs
            .iter()
            .position(|tab| tab.contains(pane))
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))
    }

    pub(crate) fn tab_mut(
        &mut self,
        id: TabId,
    ) -> Result<&mut Tab> {
        self.tabs
            .iter_mut()
            .find(|tab| tab.id() == id)
            .ok_or(Error::TabNotFound(id))
    }

    pub(crate) fn tab_of_mut(
        &mut self,
        pane: &PaneId,
    ) -> Result<&mut Tab> {
        self.tabs
            .iter_mut()
            .find(|tab| tab.contains(pane))
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))
    }

    pub(crate) fn take_tab_at(
        &mut self,
        index: usize,
    ) -> Option<Tab> {
        if index >= self.tabs.len() {
            return None;
        }
        let removed = self.tabs.remove(index);
        if self.shown.get(&removed.space) != Some(&removed.id()) {
            return Some(removed);
        }
        let same_space = |tab: &&Tab| tab.space == removed.space;
        let after = self.tabs.iter().skip(index).find(same_space);
        let heir = after.or_else(|| self.tabs.iter().take(index).rfind(same_space));
        match heir.map(Tab::id) {
            Some(heir) => self.shown.insert(removed.space.clone(), heir),
            None => self.shown.remove(&removed.space),
        };
        Some(removed)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::layout_fixtures::sample;
    use crate::test_support::tab_fixtures::id;
    use crate::tree::split_node::SplitNode;

    #[test]
    fn an_arrangement_naming_a_vanished_pane_is_refused_untouched() {
        let mut layout = sample();
        let before = layout.clone();
        let mut arrangement = layout.arrangement();
        arrangement.tabs[0].tree = Some(SplitNode::leaf(id("ghost")));
        let step = UndoStep {
            arrangement,
            parked: BTreeMap::new(),
            spaces: std::collections::BTreeSet::new(),
        };
        assert!(layout.swap_arrangement(step).is_err(), "refused");
        assert_eq!(layout, before, "layout untouched");
    }
}
