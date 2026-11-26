use std::cell::RefCell;
use std::collections::BTreeSet;
use std::rc::Rc;

use protocol::PaneId;

#[derive(Debug, Clone, Default)]
pub(crate) struct DirtyPanes {
    panes: Rc<RefCell<BTreeSet<PaneId>>>,
}

impl DirtyPanes {
    pub(crate) fn mark(
        &self,
        pane: PaneId,
    ) {
        self.panes.borrow_mut().insert(pane);
    }

    pub(crate) fn clear(
        &self,
        pane: &PaneId,
    ) {
        self.panes.borrow_mut().remove(pane);
    }

    pub(crate) fn contains(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.panes.borrow().contains(pane)
    }

    pub(crate) fn all(&self) -> Vec<PaneId> {
        self.panes.borrow().iter().cloned().collect()
    }

    pub(crate) fn retain(
        &self,
        keep: impl Fn(&PaneId) -> bool,
    ) {
        self.panes.borrow_mut().retain(|pane| keep(pane));
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn clones_share_the_flags_and_a_clean_report_clears_one() {
        let dirty = DirtyPanes::default();
        let copy = dirty.clone();
        let pane = PaneId::new("a").unwrap();
        dirty.mark(pane.clone());
        assert!(
            copy.contains(&pane) && copy.all() == [pane.clone()],
            "shared"
        );
        copy.clear(&pane);
        assert!(!dirty.contains(&pane) && dirty.all().is_empty(), "cleared");
    }

    #[test]
    fn retain_forgets_panes_that_are_gone() {
        let dirty = DirtyPanes::default();
        let (kept, gone) = (PaneId::new("a").unwrap(), PaneId::new("b").unwrap());
        dirty.mark(kept.clone());
        dirty.mark(gone.clone());
        dirty.retain(|pane| *pane == kept);
        assert!(dirty.contains(&kept) && !dirty.contains(&gone));
    }
}
