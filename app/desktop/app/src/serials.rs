use std::cell::Cell;
use std::rc::Rc;

use model::Layout;

/// Counters for the ids the shell invents: tabs and panes it opens itself. Clones share the
/// same counters.
#[derive(Debug, Clone)]
pub(crate) struct Serials {
    tab: Rc<Cell<u64>>,
    pane: Rc<Cell<u64>>,
}

impl Serials {
    /// Counters that start above every id `layout` already uses.
    pub(crate) fn above(layout: &Layout) -> Self {
        let tab = layout
            .tabs()
            .iter()
            .map(|tab| tab.id().0)
            .max()
            .unwrap_or(0);
        let pane = layout
            .tabs()
            .iter()
            .flat_map(model::Tab::pane_ids)
            .filter_map(|id| {
                id.as_str()
                    .strip_prefix('p')
                    .and_then(|n| n.parse::<u64>().ok())
            })
            .max()
            .unwrap_or(0);
        Self {
            tab: Rc::new(Cell::new(tab)),
            pane: Rc::new(Cell::new(pane)),
        }
    }

    pub(crate) fn next_tab(&self) -> model::TabId {
        self.tab.set(self.tab.get() + 1);
        model::TabId(self.tab.get())
    }

    pub(crate) fn next_pane(&self) -> String {
        self.pane.set(self.pane.get() + 1);
        format!("p{}", self.pane.get())
    }
}

#[cfg(test)]
#[path = "serials_tests.rs"]
mod tests;
