use protocol::PaneId;

use crate::tab_id::TabId;
use crate::tab_shape::TabShape;

#[derive(Debug, Clone, Default, PartialEq)]
pub(crate) struct Arrangement {
    pub(crate) tabs: Vec<TabShape>,
    pub(crate) active: Option<TabId>,
}

impl Arrangement {
    pub(crate) fn pane_ids(&self) -> Vec<PaneId> {
        let mut ids = Vec::new();
        for tree in self.tabs.iter().filter_map(|tab| tab.tree.as_ref()) {
            tree.collect_panes(&mut ids);
        }
        ids
    }
}

#[cfg(test)]
#[path = "arrangement_tests.rs"]
mod tests;
