use gpui::Context;
use layout::Tab;
use settings::{Key, Read as _};
use viewtabs::TabView;

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
        let working = |chat: &str| self.data.chat_working(chat);
        let tabs: Vec<TabView> = layout
            .tabs()
            .iter()
            .filter(|tab| scope.lists(project.as_deref(), &self.tab_projects(tab)))
            .map(|tab| TabView {
                id: tab.id(),
                members: crate::stack_policy::listed(tab)
                    .iter()
                    .map(|pane| member((&titles, layout), &working, pane))
                    .collect(),
            })
            .collect();
        let active = layout.active_tab().map(Tab::id);
        self.tabs
            .update(cx, |bar, cx| bar.set_tabs(tabs, active, cx));
    }

    fn tab_scope(&mut self) -> TabScope {
        let stored = self
            .settings_backend()
            .and_then(|backend| backend.get(&Key::new(TAB_LIST_SCOPE)));
        TabScope::from_setting(stored.as_ref().and_then(settings::Value::as_text))
    }

    fn tab_projects(
        &self,
        tab: &Tab,
    ) -> Vec<String> {
        projects_of(tab, |chat| self.data.chat_project(chat))
    }
}

fn projects_of(
    tab: &Tab,
    project_of: impl Fn(&str) -> Option<String>,
) -> Vec<String> {
    tab.pane_ids()
        .iter()
        .filter_map(|pane| tab.pane(pane)?.params.get("threadId"))
        .filter_map(project_of)
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn pane(
        id: &str,
        params: protocol::Params,
    ) -> layout::PaneInstance {
        layout::PaneInstance {
            pane_id: protocol::PaneId::new(id).unwrap(),
            type_id: protocol::PaneTypeId::new(crate::core_panes::CHAT).unwrap(),
            params,
            state: protocol::PaneState::default(),
        }
    }

    fn open(
        layout: &mut layout::Layout,
        tab: layout::TabId,
        instance: layout::PaneInstance,
    ) {
        layout
            .apply(layout::Command::OpenPane {
                tab,
                instance,
                placement: None,
            })
            .unwrap();
    }

    #[test]
    fn a_tab_belongs_to_the_projects_of_the_chats_its_panes_hold() {
        let tab = layout::TabId(1);
        let mut layout = layout::Layout::default();
        layout
            .apply(layout::Command::AddTab {
                id: tab,
                title: "t".into(),
            })
            .unwrap();
        let held = [
            pane(
                "c1",
                protocol::Params::default().with("threadId", "a").unwrap(),
            ),
            pane(
                "c2",
                protocol::Params::default()
                    .with("threadId", "unknown")
                    .unwrap(),
            ),
            pane("c3", protocol::Params::default()),
        ];
        for instance in held {
            open(&mut layout, tab, instance);
        }
        let found = layout.tabs().iter().find(|held| held.id() == tab).unwrap();
        let projects = projects_of(found, |chat| (chat == "a").then(|| "p1".to_owned()));
        assert_eq!(projects, ["p1"]);
    }
}
