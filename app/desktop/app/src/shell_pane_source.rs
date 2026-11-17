use gpui::{AnyElement, AnyView, App, SharedString, Window};
use model::TabId;
use protocol::PaneId;

use crate::browser_bars::BrowserBars;
use crate::native_panes::NativePanes;
use crate::serials::Serials;
use crate::titles::Titles;

pub(crate) struct ShellPaneSource {
    titles: Titles,
    serials: Serials,
    bars: BrowserBars,
    native: NativePanes,
}

impl ShellPaneSource {
    pub(crate) fn new(
        titles: Titles,
        serials: Serials,
        bars: BrowserBars,
        native: NativePanes,
    ) -> Self {
        Self {
            titles,
            serials,
            bars,
            native,
        }
    }

    pub(crate) fn native(&self) -> &NativePanes {
        &self.native
    }

    pub(crate) fn bars(&self) -> &BrowserBars {
        &self.bars
    }
}

impl layout_view::PaneSource for ShellPaneSource {
    fn content(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        self.native.content(pane, window, cx)
    }

    fn is_native(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.native.is_native(pane)
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

    fn tab_look(
        &self,
        instance: &model::PaneInstance,
    ) -> layout_view::TabLook {
        let title = self
            .titles
            .get(&instance.pane_id)
            .map(|title| title.to_string());
        crate::tab_looks::tab_look(instance, title.as_deref())
    }

    fn stacks(
        &self,
        instance: &model::PaneInstance,
    ) -> bool {
        crate::stack_policy::stacks(&instance.type_id)
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
    use crate::new_tab_panes::NewTabPanes;
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
            NativePanes::new(
                idle_terminals(TerminalIds::default()),
                NewTabPanes::new(async_channel::unbounded().0),
            ),
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
            NativePanes::new(
                idle_terminals(TerminalIds::default()),
                NewTabPanes::new(async_channel::unbounded().0),
            ),
        );
        let pane = PaneId::new("p1").unwrap();
        let (first, _) = source.fresh_tab(&pane);
        let (second, _) = source.fresh_tab(&pane);
        assert_ne!(first, second, "never the same id twice");
    }
}
