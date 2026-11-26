use gpui::{AnyElement, App, SharedString};
use layout::TabId;
use protocol::PaneId;

use super::{PanePreviews, ShellPaneSource};
use crate::panes::kinds::chat_label;

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
            || chat_label::NEW_CHAT.into(),
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
    use crate::panes::content::shell_pane_source::tests::idle_source;
    use crate::panes::tracking::titles::Titles;

    #[test]
    fn an_unnamed_chat_is_called_new_chat_and_a_named_one_by_its_title() {
        let titles = Titles::default();
        let source = idle_source(titles.clone());
        let pane = PaneId::new("p1").unwrap();
        assert_eq!(source.label(&pane).as_ref(), "New chat", "unnamed");
        titles.set(pane.clone(), PaneTitle::new("Fix").unwrap());
        assert_eq!(source.label(&pane).as_ref(), "Fix", "named");
    }

    #[test]
    fn detached_panes_get_fresh_tabs() {
        let source = idle_source(Titles::default());
        let pane = PaneId::new("p1").unwrap();
        let (first, _) = source.fresh_tab(&pane);
        let (second, _) = source.fresh_tab(&pane);
        assert_ne!(first, second, "never the same id twice");
    }
}
