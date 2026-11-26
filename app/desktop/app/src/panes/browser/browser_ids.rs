use std::cell::RefCell;
use std::collections::BTreeSet;
use std::rc::Rc;

use layout::{Layout, Tab};
use protocol::PaneId;

#[derive(Debug, Clone, Default)]
pub(crate) struct BrowserIds {
    panes: Rc<RefCell<BTreeSet<PaneId>>>,
}

impl BrowserIds {
    pub(crate) fn contains(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.panes.borrow().contains(pane)
    }

    pub(crate) fn follow(
        &self,
        layout: &Layout,
    ) {
        let browsers: BTreeSet<PaneId> = layout
            .tabs()
            .iter()
            .flat_map(Tab::pane_ids)
            .filter(|pane| is_browser(layout, pane))
            .collect();
        *self.panes.borrow_mut() = browsers;
    }
}

fn is_browser(
    layout: &Layout,
    pane: &PaneId,
) -> bool {
    let type_id = layout.pane(pane).map(|instance| instance.type_id.as_str());
    type_id == Some(browser::BROWSER_PANE_TYPE)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{one_tab, pane};

    #[test]
    fn only_browser_panes_of_the_layout_are_listed() {
        let layout = one_tab([pane("p1", "core.browser"), pane("p2", "core.chat")]);
        let ids = BrowserIds::default();
        ids.follow(&layout);
        assert!(ids.contains(&PaneId::new("p1").unwrap()));
        assert!(!ids.contains(&PaneId::new("p2").unwrap()));
    }
}
