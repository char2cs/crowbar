use std::collections::BTreeMap;

use protocol::PaneId;

use crate::arrangement::Arrangement;
use crate::error::{Error, Result};
use crate::layout::Layout;
use crate::pane_instance::PaneInstance;
use crate::tab::Tab;
use crate::tab_id::TabId;
use crate::undo_step::UndoStep;

impl Layout {
    pub(crate) fn arrangement(&self) -> Arrangement {
        Arrangement {
            tabs: self.tabs.iter().map(Tab::shape).collect(),
            active: self.active,
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
        let Arrangement { tabs, active } = step.arrangement;
        self.tabs = tabs
            .into_iter()
            .map(|shape| Tab::from_shape(shape, &mut pool))
            .collect();
        self.active = active;
        Ok(UndoStep {
            arrangement: before,
            parked: pool,
        })
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
        if self.active == Some(removed.id()) {
            let heir = self.tabs.get(index).or_else(|| self.tabs.last());
            self.active = heir.map(Tab::id);
        }
        Some(removed)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::command::Command;
    use crate::split_node::SplitNode;
    use crate::test_support::layout_fixtures::sample;
    use crate::test_support::tab_fixtures::id;

    #[test]
    fn swapping_in_an_arrangement_keeps_live_panes_and_parks_the_rest() {
        let mut layout = sample();
        let before = layout.arrangement();
        let removed = layout
            .execute(Command::ClosePane { pane: id("b") })
            .unwrap();
        let step = UndoStep {
            arrangement: before.clone(),
            parked: removed
                .into_iter()
                .map(|p| (p.pane_id.clone(), p))
                .collect(),
        };
        let after_close = layout.arrangement();
        let inverse = layout.swap_arrangement(step).unwrap();
        assert_eq!(layout.arrangement(), before, "arrangement back");
        assert_eq!(layout.pane_count(), 3, "pane back");
        assert!(
            inverse.parked.is_empty() && inverse.arrangement == after_close,
            "inverse: {inverse:?}"
        );
        layout.validate().unwrap();
    }

    #[test]
    fn an_arrangement_naming_a_vanished_pane_is_refused_untouched() {
        let mut layout = sample();
        let before = layout.clone();
        let mut arrangement = layout.arrangement();
        arrangement.tabs[0].tree = Some(SplitNode::leaf(id("ghost")));
        let step = UndoStep {
            arrangement,
            parked: BTreeMap::new(),
        };
        assert!(layout.swap_arrangement(step).is_err(), "refused");
        assert_eq!(layout, before, "layout untouched");
    }
}
