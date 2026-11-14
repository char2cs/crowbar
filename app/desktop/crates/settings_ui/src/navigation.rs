//! The selected page and keyboard movement along the rail.

use crate::page_id::PageId;
use crate::rail_move::RailMove;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Navigation {
    active: PageId,
}

impl Navigation {
    pub(crate) fn new(active: PageId) -> Self {
        Self { active }
    }

    pub(crate) fn active(&self) -> &PageId {
        &self.active
    }

    pub(crate) fn select(&mut self, id: &PageId, visible: &[&PageId]) {
        if visible.contains(&id) {
            self.active = id.clone();
        }
    }

    pub(crate) fn step(&mut self, movement: RailMove, visible: &[&PageId]) {
        let Some(last) = visible.len().checked_sub(1) else {
            return;
        };
        let current = visible.iter().position(|id| **id == self.active);
        let target = match (movement, current) {
            (RailMove::First, _) | (RailMove::Next, None) => 0,
            (RailMove::Last, _) | (RailMove::Previous, None | Some(0)) => last,
            (RailMove::Next, Some(at)) if at >= last => 0,
            (RailMove::Next, Some(at)) => at + 1,
            (RailMove::Previous, Some(at)) => at - 1,
        };
        if let Some(id) = visible.get(target) {
            self.active = (*id).clone();
        }
    }

    pub(crate) fn reconcile(&mut self, visible: &[&PageId]) {
        if visible.contains(&&self.active) {
            return;
        }
        if let Some(first) = visible.first() {
            self.active = (*first).clone();
        }
    }
}

#[cfg(test)]
#[path = "navigation_tests.rs"]
mod tests;
