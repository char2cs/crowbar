use std::cell::Cell;
use std::rc::Rc;

use model::Layout;

#[derive(Debug, Clone)]
pub(crate) struct Serials {
    tab: Rc<Cell<u64>>,
    pane: Rc<Cell<u64>>,
}

impl Serials {
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
mod tests {
    use model::{Command, PaneInstance, TabId};
    use protocol::{PaneId, PaneState, PaneTypeId, Params};

    use super::*;

    fn chat_instance(id: &str) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new(id).unwrap(),
            type_id: PaneTypeId::new("core.chat").unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        }
    }

    #[test]
    fn an_empty_layout_starts_at_one() {
        let serials = Serials::above(&Layout::default());
        assert_eq!(serials.next_tab(), TabId(1), "tab");
        assert_eq!(serials.next_pane(), "p1", "pane");
    }

    #[test]
    fn counters_start_above_the_ids_in_use() {
        let mut layout = Layout::default();
        layout
            .apply(Command::AddTab {
                id: TabId(7),
                title: "t".into(),
            })
            .unwrap();
        layout
            .apply(Command::OpenPane {
                tab: TabId(7),
                instance: chat_instance("p12"),
                placement: None,
            })
            .unwrap();
        let serials = Serials::above(&layout);
        assert_eq!(serials.next_tab(), TabId(8), "tab");
        assert_eq!(serials.next_pane(), "p13", "pane");
        assert_eq!(serials.next_pane(), "p14", "advances");
    }
}
