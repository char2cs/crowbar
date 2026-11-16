use std::cell::RefCell;
use std::collections::BTreeSet;

use pane_host::BrowseAction;
use protocol::PaneId;

#[derive(Debug, Default)]
pub(crate) struct PickingPanes {
    active: RefCell<BTreeSet<PaneId>>,
}

impl PickingPanes {
    pub(crate) fn toggle(
        &self,
        pane: &PaneId,
    ) -> BrowseAction {
        if self.active.borrow_mut().remove(pane) {
            return BrowseAction::CancelPick;
        }
        self.active.borrow_mut().insert(pane.clone());
        BrowseAction::Pick
    }

    pub(crate) fn finish(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.active.borrow_mut().remove(pane)
    }

    pub(crate) fn is_picking(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.active.borrow().contains(pane)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_pick_starts_stops_and_is_reported_only_while_pending() {
        let pane = PaneId::new("p1").unwrap();
        let other = PaneId::new("p2").unwrap();
        let picks = PickingPanes::default();
        assert!(!picks.finish(&pane), "nothing was started");
        assert_eq!(picks.toggle(&pane), BrowseAction::Pick);
        assert!(picks.is_picking(&pane));
        assert!(!picks.is_picking(&other));
        assert_eq!(picks.toggle(&pane), BrowseAction::CancelPick);
        assert!(!picks.is_picking(&pane));
        picks.toggle(&pane);
        assert!(picks.finish(&pane), "the result of a started pick counts");
        assert!(!picks.finish(&pane), "and only once");
    }
}
