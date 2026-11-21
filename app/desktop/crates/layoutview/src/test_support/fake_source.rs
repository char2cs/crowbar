use std::cell::Cell;

use gpui::{AnyElement, App, IntoElement, SharedString, Window, div};
use layout::{PaneInstance, TabId};
use protocol::PaneId;

use crate::branch_review::BranchReview;
use crate::pane_chrome::PaneChrome;
use crate::pane_content::PaneContent;
use crate::pane_detach::PaneDetach;
use ideshell::{TabLook, TabLooks};

pub(crate) struct FakeSource {
    next: Cell<u64>,
    native: Vec<PaneId>,
    chrome: Vec<(PaneId, f32)>,
    looks: Vec<(PaneId, TabLook)>,
    reviewing: Vec<PaneId>,
}

const FIRST_TAB: u64 = 100;

impl FakeSource {
    pub(crate) fn starting_at_first_tab() -> Self {
        Self {
            next: Cell::new(FIRST_TAB),
            native: Vec::new(),
            chrome: Vec::new(),
            looks: Vec::new(),
            reviewing: Vec::new(),
        }
    }

    pub(crate) fn native(
        mut self,
        pane: PaneId,
    ) -> Self {
        self.native.push(pane);
        self
    }

    pub(crate) fn looking(
        mut self,
        pane: PaneId,
        look: TabLook,
    ) -> Self {
        self.looks.push((pane, look));
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

    pub(crate) fn reviewing(
        mut self,
        chat: PaneId,
    ) -> Self {
        self.reviewing.push(chat);
        self
    }
}

impl BranchReview for FakeSource {
    fn reviews_branch(
        &self,
        chat: &PaneInstance,
    ) -> bool {
        self.reviewing.contains(&chat.pane_id)
    }
}

impl PaneContent for FakeSource {
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
}

impl PaneChrome for FakeSource {
    fn chrome_height(
        &self,
        pane: &PaneId,
    ) -> f32 {
        self.chrome
            .iter()
            .find(|(chromed, _)| chromed == pane)
            .map_or(0.0, |(_, height)| *height)
    }
}

impl TabLooks for FakeSource {
    fn tab_look(
        &self,
        instance: &PaneInstance,
    ) -> TabLook {
        let known = self
            .looks
            .iter()
            .find(|(pane, _)| *pane == instance.pane_id);
        known.map_or_else(
            || TabLook::plain(instance.type_id.to_string()),
            |(_, look)| look.clone(),
        )
    }
}

impl PaneDetach for FakeSource {
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
