use gpui::{AnyElement, App, SharedString};
use layout::TabId;
use protocol::PaneId;

use super::{PanePreviews, ShellPaneSource};

impl ShellPaneSource {
    pub(crate) fn previewing(
        mut self,
        previews: PanePreviews,
    ) -> Self {
        self.previews = previews;
        self
    }
}

impl layoutview::PaneDetach for ShellPaneSource {
    fn label(
        &self,
        pane: &PaneId,
    ) -> SharedString {
        self.titles.get(pane).map_or_else(
            || crate::chat_label::NEW_CHAT.into(),
            |title| title.to_string().into(),
        )
    }

    fn preview(
        &self,
        pane: &PaneId,
        cx: &App,
    ) -> Option<AnyElement> {
        (self.previews)(pane, cx)
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
    use layoutview::PaneDetach as _;
    use protocol::PaneTitle;

    use super::*;
    use crate::browser_bars::BrowserBars;
    use crate::native_panes::NativePanes;
    use crate::new_tab_panes::NewTabPanes;
    use crate::serials::Serials;
    use crate::terminal_ids::TerminalIds;
    use crate::test_support::idle_terminals;
    use crate::titles::Titles;

    fn source(titles: Titles) -> ShellPaneSource {
        let bars = BrowserBars::new(
            crate::browser_ids::BrowserIds::default(),
            async_channel::unbounded().0,
        );
        let native = NativePanes::new(
            idle_terminals(TerminalIds::default()),
            NewTabPanes::new(async_channel::unbounded().0),
            crate::test_support::idle_editors(),
        );
        let serials = Serials::above(&layout::Layout::default());
        ShellPaneSource::new(titles, serials, bars, native)
    }

    #[test]
    fn an_unnamed_chat_is_called_new_chat_and_a_named_one_by_its_title() {
        let titles = Titles::default();
        let source = source(titles.clone());
        let pane = PaneId::new("p1").unwrap();
        assert_eq!(source.label(&pane).as_ref(), "New chat", "unnamed");
        titles.set(pane.clone(), PaneTitle::new("Fix").unwrap());
        assert_eq!(source.label(&pane).as_ref(), "Fix", "named");
    }

    #[test]
    fn detached_panes_get_fresh_tabs() {
        let source = source(Titles::default());
        let pane = PaneId::new("p1").unwrap();
        let (first, _) = source.fresh_tab(&pane);
        let (second, _) = source.fresh_tab(&pane);
        assert_ne!(first, second, "never the same id twice");
    }
}
