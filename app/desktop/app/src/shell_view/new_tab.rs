use gpui::{Context, Window};
use layout::{Command, Layout, TabId};
use protocol::PaneId;

use super::ShellView;
use crate::core_panes::NEW_TAB;
use crate::focused_chat::focused_chat;
use crate::new_tab_action::NewTabAction;
use crate::pane_spec::PaneSpec;
use crate::stack_policy::stack_target;

impl ShellView {
    pub(super) fn new_tab(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let id = self.state.serials().next_tab();
        self.state.apply(stage_view(self.state.layout(), id));
        self.open_page(cx);
    }

    pub(super) fn new_tab_in(
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
        if let Some(pane) = page_in_stack(self.state.layout()) {
            self.state.apply(vec![Command::FocusPane { pane }]);
            self.refresh(cx);
            return;
        }
        let chat = focused_chat(self.state.layout());
        let page = page_name(&self.state.serials().next_pane());
        self.open_spec(PaneSpec::new_tab(&page, chat.as_deref()), cx);
    }

    pub(super) fn choose_in_new_tab(
        &mut self,
        (page, action): (&PaneId, NewTabAction),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let before = self.state.layout().pane_count();
        self.run_shell_command(action.command(), window, cx);
        if opened_pane(before, self.state.layout().pane_count()) {
            self.state
                .apply(vec![Command::ClosePane { pane: page.clone() }]);
            self.refresh(cx);
        }
    }
}

fn stage_view(
    layout: &Layout,
    id: TabId,
) -> Vec<Command> {
    if layout.active_tab().is_some() {
        return Vec::new();
    }
    vec![Command::AddTab {
        id,
        title: String::new(),
    }]
}

fn page_in_stack(layout: &Layout) -> Option<PaneId> {
    let tab = layout.active_tab()?;
    let group = tab.group_of(&stack_target(tab)?)?;
    let is_page = |pane: &&PaneId| {
        tab.pane(pane)
            .is_some_and(|found| found.type_id.as_str() == NEW_TAB)
    };
    group.panes().iter().find(is_page).cloned()
}

fn page_name(serial: &str) -> String {
    format!("n{serial}")
}

const fn opened_pane(
    before: usize,
    after: usize,
) -> bool {
    after > before
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::stack_policy::place;

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
            stage_view(&layout, TabId(1)),
            [Command::AddTab {
                id: TabId(1),
                title: String::new(),
            }],
            "the stage gets a view to hold the page"
        );
        layout
            .apply(stage_view(&layout, TabId(1)).remove(0))
            .unwrap();
        assert!(
            stage_view(&layout, TabId(2)).is_empty(),
            "a showing view takes the page in place"
        );
    }

    #[test]
    fn a_view_keeps_one_new_tab_page_in_its_stack() {
        let mut layout = Layout::default();
        layout
            .apply(stage_view(&layout, TabId(1)).remove(0))
            .unwrap();
        assert_eq!(page_in_stack(&layout), None, "an empty view has none");
        for (name, kind) in [("c1", "core.chat"), ("e1", "core.editor"), ("n1", NEW_TAB)] {
            let instance = instance(name, kind);
            layout
                .apply(place(layout.active_tab().unwrap(), instance))
                .unwrap();
        }
        let page = page_in_stack(&layout).map(|pane| pane.as_str().to_owned());
        assert_eq!(page.as_deref(), Some("n1"), "the stack's page is found");
    }

    #[test]
    fn a_choice_replaces_its_page_only_when_a_pane_opened() {
        const CASES: &[(usize, usize, bool)] =
            &[(1, 2, true), (3, 3, false), (3, 2, false), (0, 1, true)];
        for (before, after, opened) in CASES {
            assert_eq!(opened_pane(*before, *after), *opened, "{before} -> {after}");
        }
    }
}
