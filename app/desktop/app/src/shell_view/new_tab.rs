use gpui::{Context, Window};
use model::Command;
use protocol::PaneId;

use super::ShellView;
use crate::focused_chat::focused_chat;
use crate::new_tab_action::NewTabAction;
use crate::pane_spec::PaneSpec;

impl ShellView {
    pub(super) fn new_tab(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let chat = focused_chat(self.state.layout());
        let id = self.state.serials().next_tab();
        self.state.apply(vec![Command::AddTab {
            id,
            title: String::new(),
        }]);
        let page = page_name(&self.state.serials().next_pane());
        self.open_spec(PaneSpec::new_tab(&page, chat.as_deref()), cx);
    }

    pub(super) fn new_tab_in(
        &mut self,
        onto: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        self.state
            .apply(vec![Command::FocusPane { pane: onto.clone() }]);
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

    #[test]
    fn a_page_is_named_after_the_pane_serial() {
        const CASES: &[(&str, &str)] = &[("p1", "np1"), ("p42", "np42")];
        for (serial, name) in CASES {
            assert_eq!(page_name(serial), *name);
        }
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
