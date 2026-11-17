use gpui::{AnyElement, App, IntoElement as _, Window, div};
use protocol::PaneId;

use crate::new_tab_panes::NewTabPanes;
use crate::terminal_panes::TerminalPanes;
use crate::titles::Titles;

pub(crate) struct NativePanes {
    terminals: TerminalPanes,
    new_tabs: NewTabPanes,
}

impl NativePanes {
    pub(crate) fn new(
        terminals: TerminalPanes,
        new_tabs: NewTabPanes,
    ) -> Self {
        Self {
            terminals,
            new_tabs,
        }
    }

    pub(crate) fn follow(
        &self,
        layout: &model::Layout,
        titles: &Titles,
    ) {
        self.new_tabs.follow(layout, titles);
    }

    pub(crate) fn terminals_attached(&self) -> bool {
        self.terminals.attached()
    }

    pub(crate) fn apply_terminal_settings(
        &self,
        settings: terminal_view::TerminalSettings,
        cx: &mut App,
    ) {
        self.terminals.apply(settings, cx);
    }

    pub(crate) fn is_native(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.terminals.is_terminal(pane) || self.new_tabs.contains(pane)
    }

    pub(crate) fn content(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        if let Some(page) = self.new_tabs.view(pane, cx) {
            return page.into_any_element();
        }
        match self.terminals.view(pane, window, cx) {
            Some(view) => view.into_any_element(),
            None => div().into_any_element(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::terminal_ids::TerminalIds;
    use crate::test_support::idle_terminals;

    fn native() -> NativePanes {
        NativePanes::new(
            idle_terminals(TerminalIds::default()),
            NewTabPanes::new(async_channel::unbounded().0),
        )
    }

    #[test]
    fn a_pane_nobody_registered_is_a_web_pane() {
        let pane = PaneId::new("p1").unwrap();
        assert!(!native().is_native(&pane), "unknown pane");
    }

    #[test]
    fn idle_terminals_are_not_attached() {
        assert!(!native().terminals_attached(), "nothing attached");
    }
}
