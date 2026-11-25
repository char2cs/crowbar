use std::cell::Cell;
use std::rc::Rc;

use layout::Layout;

const PANE_PREFIX: char = 'p';
const BROWSER_PREFIX: char = 'b';

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
            .flat_map(layout::Tab::pane_ids)
            .filter_map(|id| serial_of(id.as_str()))
            .max()
            .unwrap_or(0);
        Self {
            tab: Rc::new(Cell::new(tab)),
            pane: Rc::new(Cell::new(pane)),
        }
    }

    pub(crate) fn next_tab(&self) -> layout::TabId {
        self.tab.set(self.tab.get() + 1);
        layout::TabId(self.tab.get())
    }

    pub(crate) fn next_pane(&self) -> String {
        format!("{PANE_PREFIX}{}", self.advance())
    }

    pub(crate) fn next_browser(&self) -> String {
        format!("{BROWSER_PREFIX}{}", self.advance())
    }

    fn advance(&self) -> u64 {
        self.pane.set(self.pane.get() + 1);
        self.pane.get()
    }
}

fn serial_of(id: &str) -> Option<u64> {
    let number = id
        .strip_prefix(PANE_PREFIX)
        .or_else(|| id.strip_prefix(BROWSER_PREFIX))?;
    number.parse().ok()
}

#[cfg(test)]
mod tests {
    use layout::{Command, PaneInstance, TabId};
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

    struct Case {
        what: &'static str,
        ids: &'static [&'static str],
        next_tab: u64,
        next: [&'static str; 3],
    }

    const CASES: &[Case] = &[
        Case {
            what: "an empty layout starts at one",
            ids: &[],
            next_tab: 1,
            next: ["p1", "b2", "p3"],
        },
        Case {
            what: "counters start above the pane ids in use",
            ids: &["p12"],
            next_tab: 8,
            next: ["p13", "b14", "p15"],
        },
        Case {
            what: "a browser id counts when the layout is read back",
            ids: &["b9"],
            next_tab: 8,
            next: ["p10", "b11", "p12"],
        },
    ];

    fn open_command(
        tab: TabId,
        id: &str,
    ) -> Command {
        Command::OpenPane {
            tab,
            instance: chat_instance(id),
            placement: None,
        }
    }

    fn layout_of(ids: &[&str]) -> Layout {
        let mut layout = Layout::default();
        if ids.is_empty() {
            return layout;
        }
        let tab = TabId(7);
        let title = "t".into();
        layout
            .apply(Command::AddTab {
                id: tab,
                title,
                space: layout::SpaceId::default(),
            })
            .unwrap();
        for id in ids {
            layout.apply(open_command(tab, id)).unwrap();
        }
        layout
    }

    #[test]
    fn ids_continue_above_the_ones_in_use_and_browsers_share_the_pane_counter() {
        for case in CASES {
            let serials = Serials::above(&layout_of(case.ids));
            let tab = TabId(case.next_tab);
            assert_eq!(serials.next_tab(), tab, "{}: tab", case.what);
            let heard = [
                serials.next_pane(),
                serials.next_browser(),
                serials.next_pane(),
            ];
            assert_eq!(heard, case.next, "{}", case.what);
        }
    }
}
