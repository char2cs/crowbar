use std::cell::RefCell;
use std::collections::BTreeMap;
use std::rc::Rc;

use protocol::{PaneId, PaneTitle};

/// The titles panes gave themselves. Clones share one table, so the tab strip and the shell
/// state read the same titles.
#[derive(Debug, Clone, Default)]
pub(crate) struct Titles {
    entries: Rc<RefCell<BTreeMap<PaneId, PaneTitle>>>,
}

impl Titles {
    pub(crate) fn set(&self, pane: PaneId, title: PaneTitle) {
        self.entries.borrow_mut().insert(pane, title);
    }

    pub(crate) fn get(&self, pane: &PaneId) -> Option<PaneTitle> {
        self.entries.borrow().get(pane).cloned()
    }

    /// Forgets the titles of panes that no longer satisfy `keep`.
    pub(crate) fn retain(&self, keep: impl Fn(&PaneId) -> bool) {
        self.entries.borrow_mut().retain(|pane, _| keep(pane));
    }
}

#[cfg(test)]
#[path = "titles_tests.rs"]
mod tests;
