use gpui::{AnyElement, AnyView, App, IntoElement as _, SharedString, Window, div};
use model::TabId;
use protocol::PaneId;

use crate::browser_bars::BrowserBars;
use crate::serials::Serials;
use crate::terminal_panes::TerminalPanes;
use crate::titles::Titles;

pub(crate) struct ShellPaneSource {
    titles: Titles,
    serials: Serials,
    bars: BrowserBars,
    terminals: TerminalPanes,
}

impl ShellPaneSource {
    pub(crate) fn new(
        titles: Titles,
        serials: Serials,
        bars: BrowserBars,
        terminals: TerminalPanes,
    ) -> Self {
        Self {
            titles,
            serials,
            bars,
            terminals,
        }
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

    pub(crate) fn show_address(
        &self,
        pane: &PaneId,
        address: &str,
        cx: &mut App,
    ) {
        self.bars.show_address(pane, address, cx);
    }
}

impl layout_view::PaneSource for ShellPaneSource {
    fn content(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        match self.terminals.view(pane, window, cx) {
            Some(view) => view.into_any_element(),
            None => div().into_any_element(),
        }
    }

    fn is_native(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.terminals.is_terminal(pane)
    }

    fn chrome_height(
        &self,
        pane: &PaneId,
    ) -> f32 {
        self.bars.height(pane)
    }

    fn chrome(
        &self,
        pane: &PaneId,
        _window: &mut Window,
        cx: &mut App,
    ) -> Option<AnyView> {
        self.bars.view(pane, cx)
    }

    fn label(
        &self,
        pane: &PaneId,
    ) -> SharedString {
        self.titles.get(pane).map_or_else(
            || crate::chat_label::NEW_CHAT.into(),
            |title| title.to_string().into(),
        )
    }

    fn fresh_tab(
        &self,
        pane: &PaneId,
    ) -> (TabId, String) {
        (self.serials.next_tab(), self.label(pane).to_string())
    }
}

#[cfg(test)]
mod tests {
    use layout_view::PaneSource as _;
    use protocol::PaneTitle;

    use super::*;
    use crate::terminal_ids::TerminalIds;
    use crate::test_support::idle_terminals;

    fn no_bars() -> BrowserBars {
        BrowserBars::new(
            crate::browser_ids::BrowserIds::default(),
            async_channel::unbounded().0,
        )
    }

    #[test]
    fn an_unnamed_chat_is_called_new_chat_and_a_named_one_by_its_title() {
        let titles = Titles::default();
        let source = ShellPaneSource::new(
            titles.clone(),
            Serials::above(&model::Layout::default()),
            no_bars(),
            idle_terminals(TerminalIds::default()),
        );
        let pane = PaneId::new("p1").unwrap();
        assert_eq!(source.label(&pane).as_ref(), "New chat", "unnamed");
        titles.set(pane.clone(), PaneTitle::new("Fix").unwrap());
        assert_eq!(source.label(&pane).as_ref(), "Fix", "named");
    }

    #[test]
    fn detached_panes_get_fresh_tabs() {
        let source = ShellPaneSource::new(
            Titles::default(),
            Serials::above(&model::Layout::default()),
            no_bars(),
            idle_terminals(TerminalIds::default()),
        );
        let pane = PaneId::new("p1").unwrap();
        let (first, _) = source.fresh_tab(&pane);
        let (second, _) = source.fresh_tab(&pane);
        assert_ne!(first, second, "never the same id twice");
    }
}
