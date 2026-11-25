//! The shell commands that start a chat in the active project.

use gpui::Context;
use sidebar::{ActiveSpace as _, PaneFocus as _, Sink as _};

use super::ShellView;
use crate::logged::emit;

mod new_chat_event;
use crate::panes::tracking::group_focus;
use new_chat_event::NewChatEvent;

impl ShellView {
    pub(super) fn new_chat(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if group_focus::editor_focused(self.state.layout(), &self.state.space()) {
            return self.new_tab(cx);
        }
        self.start_chat(sidebar::Command::NewChat);
    }

    pub(super) fn new_terminal_chat(&self) {
        let terminal_ok = self.data.view().can_start_terminal_thread();
        if terminal_ok {
            return self.start_chat(sidebar::Command::NewTerminalThread);
        }
        self.start_chat(sidebar::Command::NewChat);
    }

    fn start_chat(
        &self,
        create: fn(sidebar::ProjectId) -> sidebar::Command,
    ) {
        let Some(project) = self.data.view().active_project() else {
            return emit(&NewChatEvent::NeedsProject);
        };
        if let Err(error) = self.data.commands().dispatch(create(project)) {
            emit(&NewChatEvent::Refused { error: &error });
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use keymap::ShellCommand;
    use protocol::PaneId;

    use crate::panes::kinds::core_panes::CHAT;
    use crate::shell_view::test_support::panes_of;
    use crate::shell_view::tests::{Opened, open_shell};

    fn chats_after(
        opened: &mut Opened,
        command: ShellCommand,
    ) -> Vec<PaneId> {
        let shell = opened.shell.clone();
        shell.update_in(&mut opened.window, |shell, window, cx| {
            shell.run_shell_command(command, window, cx);
            panes_of(shell, CHAT)
        })
    }

    #[gpui::test]
    fn a_new_chat_without_a_selected_project_leaves_the_tab_as_it_is(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let shell = opened.shell.clone();
        let before = shell.read_with(&opened.window, |shell, _| panes_of(shell, CHAT));
        for command in [ShellCommand::NewChat, ShellCommand::NewChatTerminal] {
            assert_eq!(chats_after(&mut opened, command), before, "{command:?}");
        }
    }
}
