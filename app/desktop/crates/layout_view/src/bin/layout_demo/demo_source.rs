use std::cell::Cell;

use gpui::{AnyElement, App, IntoElement, ParentElement, SharedString, Styled, Window, div, px};
use layout_view::{PaneChrome, PaneContent, PaneDetach, PaneTabs};
use model::TabId;
use protocol::PaneId;
use ui::{Surface, Theme, text_size};

pub(crate) struct DemoSource {
    next_tab: Cell<u64>,
}

const FIRST_TAB: u64 = 10;
const CONTENT_PADDING: f32 = 16.0;

impl DemoSource {
    pub(crate) fn starting_at_first_tab() -> Self {
        Self {
            next_tab: Cell::new(FIRST_TAB),
        }
    }
}

impl PaneContent for DemoSource {
    fn content(
        &self,
        pane: &PaneId,
        _window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        div()
            .size_full()
            .p(px(CONTENT_PADDING))
            .text_size(text_size::SM)
            .text_color(palette.muted_foreground())
            .child(format!(
                "pane {} (the web view fills this hole)",
                pane.as_str()
            ))
            .into_any_element()
    }
}

impl PaneChrome for DemoSource {}

impl PaneTabs for DemoSource {}

impl PaneDetach for DemoSource {
    fn label(
        &self,
        pane: &PaneId,
    ) -> SharedString {
        format!("Chat {}", pane.as_str()).into()
    }

    fn fresh_tab(
        &self,
        _pane: &PaneId,
    ) -> (TabId, String) {
        let id = self.next_tab.get();
        self.next_tab.set(id + 1);
        (TabId(id), "Detached".into())
    }
}
