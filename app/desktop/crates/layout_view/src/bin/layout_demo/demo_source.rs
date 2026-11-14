use std::cell::Cell;

use gpui::{AnyElement, App, IntoElement, ParentElement, SharedString, Styled, Window, div, px};
use layout_view::PaneSource;
use model::TabId;
use protocol::PaneId;
use ui::{Surface, Theme, text_size};

pub(crate) struct DemoSource {
    next_tab: Cell<u64>,
}

impl DemoSource {
    pub(crate) fn new() -> Self {
        Self {
            next_tab: Cell::new(10),
        }
    }
}

impl PaneSource for DemoSource {
    fn content(&self, pane: &PaneId, _window: &mut Window, cx: &mut App) -> AnyElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        div()
            .size_full()
            .p(px(16.0))
            .text_size(text_size::SM)
            .text_color(palette.muted_foreground())
            .child(format!(
                "pane {} (the web view fills this hole)",
                pane.as_str()
            ))
            .into_any_element()
    }

    fn label(&self, pane: &PaneId) -> SharedString {
        format!("Chat {}", pane.as_str()).into()
    }

    fn fresh_tab(&self, _pane: &PaneId) -> (TabId, String) {
        let id = self.next_tab.get();
        self.next_tab.set(id + 1);
        (TabId(id), "Detached".into())
    }
}
