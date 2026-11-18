use std::collections::BTreeMap;

use browser::{PageState, PageUpdate};
use model::Layout;
use protocol::PaneId;

#[derive(Debug, Default)]
pub(crate) struct BrowserPages {
    states: BTreeMap<PaneId, PageState>,
}

impl BrowserPages {
    pub(crate) fn apply(
        &mut self,
        pane: PaneId,
        update: PageUpdate,
    ) {
        self.states.entry(pane).or_default().apply(update);
    }

    pub(crate) fn get(
        &self,
        pane: &PaneId,
    ) -> Option<&PageState> {
        self.states.get(pane)
    }

    pub(crate) fn retain_open(
        &mut self,
        layout: &Layout,
    ) {
        self.states.retain(|pane, _| layout.pane(pane).is_some());
    }
}

#[cfg(test)]
mod tests {
    use model::{Command, PaneInstance, TabId};
    use protocol::{PaneState, PaneTypeId, Params};

    use super::*;

    fn pane(name: &str) -> PaneId {
        PaneId::new(name).unwrap()
    }

    fn layout_with(name: &str) -> Layout {
        let mut layout = Layout::default();
        let tab = TabId(1);
        layout
            .apply(Command::AddTab {
                id: tab,
                title: String::new(),
            })
            .unwrap();
        let instance = PaneInstance {
            pane_id: pane(name),
            type_id: PaneTypeId::new("core.browser").unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        };
        let placement = None;
        layout
            .apply(Command::OpenPane {
                tab,
                instance,
                placement,
            })
            .unwrap();
        layout
    }

    #[test]
    fn a_page_is_followed_until_its_pane_is_closed() {
        let mut pages = BrowserPages::default();
        pages.apply(
            pane("b1"),
            PageUpdate::Loading {
                url: "https://a.test/".into(),
            },
        );
        pages.apply(pane("b1"), PageUpdate::title("A"));
        pages.apply(
            pane("b2"),
            PageUpdate::Loading {
                url: "https://b.test/".into(),
            },
        );
        let seen = pages.get(&pane("b1")).map(|state| {
            (
                state.url().to_owned(),
                state.title().map(str::to_owned),
                state.is_loading(),
            )
        });
        assert_eq!(
            seen,
            Some(("https://a.test/".to_owned(), Some("A".to_owned()), true))
        );
        pages.retain_open(&layout_with("b1"));
        assert!(pages.get(&pane("b1")).is_some(), "open");
        assert!(pages.get(&pane("b2")).is_none(), "closed");
    }
}
