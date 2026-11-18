use std::cell::RefCell;
use std::collections::BTreeSet;
use std::rc::Rc;

use model::{Layout, Tab};
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
    use model::{Command, PaneInstance, TabId};
    use protocol::{PaneState, PaneTypeId, Params};

    use super::*;

    fn open(
        layout: &mut Layout,
        pane: &str,
        type_id: &str,
    ) {
        let instance = PaneInstance {
            pane_id: PaneId::new(pane).unwrap(),
            type_id: PaneTypeId::new(type_id).unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        };
        layout
            .apply(Command::OpenPane {
                tab: TabId(1),
                instance,
                placement: None,
            })
            .unwrap();
    }

    #[test]
    fn only_browser_panes_of_the_layout_are_listed() {
        let mut layout = Layout::default();
        layout
            .apply(Command::AddTab {
                id: TabId(1),
                title: "t".into(),
            })
            .unwrap();
        open(&mut layout, "p1", "core.browser");
        open(&mut layout, "p2", "core.chat");
        let ids = BrowserIds::default();
        ids.follow(&layout);
        assert!(ids.contains(&PaneId::new("p1").unwrap()));
        assert!(!ids.contains(&PaneId::new("p2").unwrap()));
    }
}
