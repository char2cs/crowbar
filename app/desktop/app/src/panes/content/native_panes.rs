use gpui::{AnyElement, App, IntoElement as _, Window, div};
use protocol::PaneId;

use crate::panes::content::new_tab::new_tab_panes::NewTabPanes;
use crate::panes::editor::editor_panes::EditorPanes;
use crate::panes::terminals::terminal_panes::TerminalPanes;
use crate::panes::tracking::titles::Titles;

pub(crate) struct NativePanes {
    terminals: TerminalPanes,
    new_tabs: NewTabPanes,
    editors: EditorPanes,
}

impl NativePanes {
    pub(crate) fn new(
        terminals: TerminalPanes,
        new_tabs: NewTabPanes,
        editors: EditorPanes,
    ) -> Self {
        Self {
            terminals,
            new_tabs,
            editors,
        }
    }

    pub(crate) fn editors(&self) -> &EditorPanes {
        &self.editors
    }

    pub(crate) fn follow(
        &self,
        (layout, space): (&layout::Layout, &str),
        titles: &Titles,
    ) {
        self.new_tabs.follow(layout, titles);
        self.editors.follow(layout, space);
    }

    pub(crate) fn apply_terminal_settings(
        &self,
        settings: terminalview::Settings,
        cx: &mut App,
    ) {
        self.terminals.apply(settings, cx);
    }

    pub(crate) fn focus(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) {
        self.terminals.focus(pane, window, cx);
        self.editors.focus(pane, window, cx);
    }

    pub(crate) fn is_native(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.terminals.is_terminal(pane) || self.new_tabs.contains(pane) || self.editors.draws(pane)
    }

    pub(crate) fn empty_editor(
        &self,
        chat: &PaneId,
        cx: &mut App,
    ) -> AnyElement {
        match self.new_tabs.empty_editor(chat, cx) {
            Some(page) => page.into_any_element(),
            None => div().into_any_element(),
        }
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
        if let Some(editor) = self.editors.content(pane, window, cx) {
            return editor;
        }
        match self.terminals.view(pane, window, cx) {
            Some(view) => view.into_any_element(),
            None => div().into_any_element(),
        }
    }
}

impl std::fmt::Debug for NativePanes {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("NativePanes")
            .finish_non_exhaustive()
    }
}

impl panehost::PaneKinds for NativePanes {
    fn kind_of(
        &self,
        pane: &PaneId,
    ) -> Option<panehost::PaneKind> {
        self.editors.kind_of(pane)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::panes::terminals::terminal_ids::TerminalIds;
    use crate::test_support::idle_terminals;

    fn native() -> NativePanes {
        NativePanes::new(
            idle_terminals(TerminalIds::default()),
            NewTabPanes::new(async_channel::unbounded().0),
            crate::test_support::idle_editors(),
        )
    }

    #[test]
    fn a_pane_nobody_registered_is_a_web_pane() {
        let pane = PaneId::new("p1").unwrap();
        assert!(!native().is_native(&pane), "unknown pane");
    }
}
