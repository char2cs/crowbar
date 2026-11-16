use std::cell::Cell;

use gpui::{AnyElement, App, IntoElement, SharedString, Window, div};
use model::{PaneInstance, TabId};
use protocol::PaneId;

use crate::pane_source::PaneSource;

pub(crate) struct FakeSource {
    next: Cell<u64>,
    native: Vec<PaneId>,
    chrome: Vec<(PaneId, f32)>,
    stacking: Vec<PaneId>,
}

const FIRST_TAB: u64 = 100;

impl FakeSource {
    pub(crate) fn starting_at_first_tab() -> Self {
        Self {
            next: Cell::new(FIRST_TAB),
            native: Vec::new(),
            chrome: Vec::new(),
            stacking: Vec::new(),
        }
    }

    pub(crate) fn native(
        mut self,
        pane: PaneId,
    ) -> Self {
        self.native.push(pane);
        self
    }

    pub(crate) fn stacking(
        mut self,
        pane: PaneId,
    ) -> Self {
        self.stacking.push(pane);
        self
    }

    pub(crate) fn with_chrome(
        mut self,
        pane: PaneId,
        height: f32,
    ) -> Self {
        self.chrome.push((pane, height));
        self
    }
}

impl PaneSource for FakeSource {
    fn content(
        &self,
        _pane: &PaneId,
        _window: &mut Window,
        _cx: &mut App,
    ) -> AnyElement {
        div().into_any_element()
    }

    fn is_native(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.native.contains(pane)
    }

    fn chrome_height(
        &self,
        pane: &PaneId,
    ) -> f32 {
        self.chrome
            .iter()
            .find(|(chromed, _)| chromed == pane)
            .map_or(0.0, |(_, height)| *height)
    }

    fn stacks(
        &self,
        instance: &PaneInstance,
    ) -> bool {
        self.stacking.contains(&instance.pane_id)
    }

    fn label(
        &self,
        pane: &PaneId,
    ) -> SharedString {
        pane.as_str().to_owned().into()
    }

    fn fresh_tab(
        &self,
        _pane: &PaneId,
    ) -> (TabId, String) {
        let id = self.next.get();
        self.next.set(id + 1);
        (TabId(id), "Detached".into())
    }
}
