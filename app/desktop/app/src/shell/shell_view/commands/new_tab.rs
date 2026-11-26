use gpui::{Context, Window};
use layout::{Command, Layout, SpaceId, Tab, TabId};
use protocol::PaneId;

use crate::panes::content::new_tab::new_tab_action::NewTabAction;
use crate::panes::kinds::core_panes::NEW_TAB;
use crate::panes::kinds::pane_spec::PaneSpec;
use crate::panes::tracking::focused_chat::focused_chat;
use crate::panes::tracking::group_focus::focused_group;
use crate::shell::shell_view::ShellView;

impl ShellView {
    pub(crate) fn new_tab(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let id = self.state.serials().next_tab();
        self.state
            .apply(stage_view((self.state.layout(), &self.state.space()), id));
        self.open_page(cx);
    }

    pub(crate) fn new_tab_in(
        &mut self,
        onto: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        self.state
            .apply(vec![Command::FocusPane { pane: onto.clone() }]);
        self.open_page(cx);
    }

    fn open_page(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(pane) = self.state.showing().and_then(page_in_group) {
            self.state.apply(vec![Command::FocusPane { pane }]);
            self.refresh(cx);
            return;
        }
        let chat = focused_chat(self.state.layout(), &self.state.space());
        let page = page_name(&self.state.serials().next_pane());
        self.open_spec(PaneSpec::new_tab(&page, chat.as_deref()), cx);
    }

    pub(crate) fn choose_in_new_tab(
        &mut self,
        (page, action): (&PaneId, NewTabAction),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.state
            .apply(vec![Command::FocusPane { pane: page.clone() }]);
        let before = self.state.layout().pane_count();
        self.run_shell_command(action.command(), window, cx);
        if replaces(self.state.layout(), page, before) {
            self.state
                .apply(vec![Command::ClosePane { pane: page.clone() }]);
            self.refresh(cx);
        }
    }
}

fn stage_view(
    (layout, space): (&Layout, &str),
    id: TabId,
) -> Vec<Command> {
    if layout.showing(space).is_some() {
        return Vec::new();
    }
    vec![Command::AddTab {
        id,
        title: String::new(),
        space: SpaceId::new(space),
    }]
}

fn page_in_group(tab: &Tab) -> Option<PaneId> {
    let group = focused_group(tab)?;
    let is_page = |pane: &&PaneId| {
        tab.pane(pane)
            .is_some_and(|found| found.type_id.as_str() == NEW_TAB)
    };
    group.tabs().iter().find(is_page).cloned()
}

fn page_name(serial: &str) -> String {
    format!("n{serial}")
}

fn replaces(
    layout: &Layout,
    page: &PaneId,
    before: usize,
) -> bool {
    let is_page = layout
        .pane(page)
        .is_some_and(|found| found.type_id.as_str() == NEW_TAB);
    is_page && layout.pane_count() > before
}

#[cfg(test)]
mod tests {
    use super::*;

    fn instance(
        name: &str,
        kind: &str,
    ) -> layout::PaneInstance {
        layout::PaneInstance {
            pane_id: PaneId::new(name).unwrap(),
            type_id: protocol::PaneTypeId::new(kind).unwrap(),
            params: protocol::Params::default().with("id", name).unwrap(),
            state: protocol::PaneState::default(),
        }
    }

    #[test]
    fn a_page_is_named_after_the_pane_serial() {
        const CASES: &[(&str, &str)] = &[("p1", "np1"), ("p42", "np42")];
        for (serial, name) in CASES {
            assert_eq!(page_name(serial), *name);
        }
    }

    #[test]
    fn a_new_tab_page_opens_inside_the_showing_view_and_needs_a_view_only_on_the_stage() {
        let mut layout = Layout::default();
        assert_eq!(
            stage_view((&layout, ""), TabId(1)),
            [Command::AddTab {
                id: TabId(1),
                title: String::new(),
                space: SpaceId::default()
            }],
            "the stage gets a view to hold the page"
        );
        layout
            .apply(stage_view((&layout, ""), TabId(1)).remove(0))
            .unwrap();
        assert!(
            stage_view((&layout, ""), TabId(2)).is_empty(),
            "a showing view takes the page in place"
        );
    }

    #[test]
    fn a_group_keeps_one_new_tab_page_among_its_tabs() {
        let mut layout = Layout::default();
        layout
            .apply(stage_view((&layout, ""), TabId(1)).remove(0))
            .unwrap();
        assert_eq!(
            layout.showing("").and_then(page_in_group),
            None,
            "an empty view has none"
        );
        for (name, kind) in [("c1", "core.chat"), ("e1", "core.editor"), ("n1", NEW_TAB)] {
            let instance = instance(name, kind);
            layout
                .apply(crate::panes::kinds::pane_opener::open_in_tab(
                    layout.showing("").unwrap(),
                    instance,
                ))
                .unwrap();
        }
        let page = layout
            .showing("")
            .and_then(page_in_group)
            .map(|pane| pane.as_str().to_owned());
        assert_eq!(page.as_deref(), Some("n1"), "the group's page is found");
    }

    #[test]
    fn a_choice_replaces_a_page_once_a_pane_opened_and_never_an_empty_editors_chat() {
        let mut layout = Layout::default();
        layout
            .apply(stage_view((&layout, ""), TabId(1)).remove(0))
            .unwrap();
        for (name, kind) in [("c1", "core.chat"), ("n1", NEW_TAB)] {
            let open = crate::panes::kinds::pane_opener::open_in_tab(
                layout.showing("").unwrap(),
                instance(name, kind),
            );
            layout.apply(open).unwrap();
        }
        let cases = [("n1", 1, true), ("n1", 2, false), ("c1", 1, false)];
        for (pane, before, replaced) in cases {
            let pane = PaneId::new(pane).unwrap();
            assert_eq!(
                replaces(&layout, &pane, before),
                replaced,
                "{pane:?} after {before}"
            );
        }
    }
}
