use gpui::Context;
use model::Tab;
use settings_ui::{Key, SettingsBackend as _};
use tab_bar::TabView;

use super::ShellView;
use super::pump::member;
use crate::tab_scope::TabScope;

pub(super) const TAB_LIST_SCOPE: &str = "tabListScope";

impl ShellView {
    pub(super) fn push_tabs(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let scope = self.tab_scope();
        let project = self.data.active_project();
        let titles = self.state.titles();
        let layout = self.state.layout();
        let tabs: Vec<TabView> = layout
            .tabs()
            .iter()
            .filter(|tab| scope.lists(project.as_deref(), &self.tab_projects(tab)))
            .map(|tab| TabView::from_tab(tab, |pane| member((&titles, layout), pane)))
            .collect();
        let active = layout.active_tab().map(Tab::id);
        self.tabs
            .update(cx, |bar, cx| bar.set_tabs(tabs, active, cx));
    }

    pub(super) fn narrows_tabs(&mut self) -> bool {
        self.tab_scope() == TabScope::Project
    }

    fn tab_scope(&mut self) -> TabScope {
        let stored = self
            .settings_backend()
            .and_then(|backend| backend.get(&Key::new(TAB_LIST_SCOPE)));
        TabScope::from_setting(stored.as_ref().and_then(settings_ui::Value::as_text))
    }

    fn tab_projects(
        &self,
        tab: &Tab,
    ) -> Vec<String> {
        tab.pane_ids()
            .iter()
            .filter_map(|pane| tab.pane(pane)?.params.get("threadId"))
            .filter_map(|chat| self.data.chat_project(chat))
            .collect()
    }
}
