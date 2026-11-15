use gpui::{AnyElement, App, IntoElement as _, SharedString, Window, div};
use model::TabId;
use protocol::PaneId;

use crate::serials::Serials;
use crate::titles::Titles;

/// What the layout view asks of the shell about a pane.
pub(crate) struct ShellPaneSource {
    titles: Titles,
    serials: Serials,
}

impl ShellPaneSource {
    pub(crate) fn new(titles: Titles, serials: Serials) -> Self {
        Self { titles, serials }
    }
}

impl layout_view::PaneSource for ShellPaneSource {
    fn content(&self, _pane: &PaneId, _window: &mut Window, _cx: &mut App) -> AnyElement {
        div().into_any_element()
    }

    fn label(&self, pane: &PaneId) -> SharedString {
        self.titles.get(pane).map_or_else(
            || crate::chat_label::NEW_CHAT.into(),
            |title| title.to_string().into(),
        )
    }

    fn fresh_tab(&self, pane: &PaneId) -> (TabId, String) {
        (self.serials.next_tab(), self.label(pane).to_string())
    }
}

#[cfg(test)]
#[path = "pane_source_tests.rs"]
mod tests;
