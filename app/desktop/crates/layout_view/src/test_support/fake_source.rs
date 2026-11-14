use std::cell::Cell;

use gpui::{AnyElement, App, IntoElement, SharedString, Window, div};
use model::TabId;
use protocol::PaneId;

use crate::pane_source::PaneSource;

pub(crate) struct FakeSource {
    next: Cell<u64>,
}

impl FakeSource {
    pub(crate) fn new() -> Self {
        Self {
            next: Cell::new(100),
        }
    }
}

impl PaneSource for FakeSource {
    fn content(&self, _pane: &PaneId, _window: &mut Window, _cx: &mut App) -> AnyElement {
        div().into_any_element()
    }

    fn label(&self, pane: &PaneId) -> SharedString {
        pane.as_str().to_owned().into()
    }

    fn fresh_tab(&self, _pane: &PaneId) -> (TabId, String) {
        let id = self.next.get();
        self.next.set(id + 1);
        (TabId(id), "Detached".into())
    }
}
