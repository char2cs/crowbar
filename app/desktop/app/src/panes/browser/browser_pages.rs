use std::collections::BTreeMap;

use browser::{PageState, PageUpdate};
use layout::Layout;
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
    use super::*;
    use crate::test_support::one_tab;

    fn pane(name: &str) -> PaneId {
        PaneId::new(name).unwrap()
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
        pages.retain_open(&one_tab([crate::test_support::pane("b1", "core.browser")]));
        assert!(pages.get(&pane("b1")).is_some(), "open");
        assert!(pages.get(&pane("b2")).is_none(), "closed");
    }
}
