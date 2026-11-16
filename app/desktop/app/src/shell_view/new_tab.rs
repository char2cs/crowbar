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
        let page = format!("n{}", self.state.serials().next_pane());
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
        if self.state.layout().pane_count() > before {
            self.state
                .apply(vec![Command::ClosePane { pane: page.clone() }]);
            self.refresh(cx);
        }
    }
}
