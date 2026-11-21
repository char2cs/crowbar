use gpui::Context;
use layout::Command;
use protocol::PaneId;
use ui::{ToastContent, ToastKind};

use super::ShellView;
use crate::file_target::FileTarget;
use crate::notice_key::NoticeKey;
use crate::pane_spec::PaneSpec;
use crate::terminal_owner::TerminalOwner;

const CANNOT_OPEN: &str = "Cannot open file";

impl ShellView {
    pub(super) fn terminal_chat(
        &self,
        pane: &PaneId,
    ) -> Option<String> {
        match self.state.terminals().owner_of(pane)? {
            TerminalOwner::Chat(chat) => Some(chat),
            TerminalOwner::Home(_) => None,
        }
    }

    pub(super) fn open_printed_file(
        &mut self,
        pane: &PaneId,
        printed: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let chat = self.terminal_chat(pane);
        let workspace = chat.as_deref().and_then(|chat| self.data.chat_root(chat));
        let directory = self
            .state
            .layout()
            .pane(pane)
            .and_then(|instance| instance.state.as_value().get("directory").cloned());
        let directory = directory.as_ref().and_then(serde_json::Value::as_str);
        let root = crate::terminal_root::terminal_root(directory, workspace.as_deref());
        let Some(absolute) = terminal::resolve_file_path(printed, root.as_deref()) else {
            let detail = format!("Could not resolve {printed} to a path.");
            return self.cannot_open(detail, cx);
        };
        let remote = chat
            .as_deref()
            .and_then(|chat| self.data.chat_remote_host(chat));
        let target = FileTarget::of(&absolute, (workspace.as_deref(), remote.as_deref()));
        self.state
            .apply(vec![Command::FocusPane { pane: pane.clone() }]);
        self.open_target(chat, target, cx);
    }

    fn open_target(
        &mut self,
        chat: Option<String>,
        target: FileTarget,
        cx: &mut Context<'_, Self>,
    ) {
        let chat = chat.or_else(|| self.chat_for("a file"));
        let Some(chat) = chat else {
            return;
        };
        match target {
            FileTarget::Workspace(relative) => {
                self.open_spec(PaneSpec::editor(&chat, &relative), cx);
            },
            FileTarget::Host(absolute) => self.open_spec(PaneSpec::host_file(&chat, &absolute), cx),
            FileTarget::Outside(absolute) => {
                let detail = format!("{absolute} is outside the current workspace.");
                self.cannot_open(detail, cx);
            },
        }
    }

    fn cannot_open(
        &mut self,
        detail: String,
        cx: &mut Context<'_, Self>,
    ) {
        let content = ToastContent::new(ToastKind::Error, detail).titled(CANNOT_OPEN);
        self.show_toast(NoticeKey::Terminal, content, cx);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use terminalview::Event;

    use crate::core_panes::EDITOR;
    use crate::shell_view::test_support::{panes_of, tell, with_terminal};

    #[gpui::test]
    fn an_absolute_path_opens_as_a_file_of_this_computer(cx: &mut TestAppContext) {
        let (mut opened, terminal) = with_terminal(cx);
        let path = "/etc/hosts".to_owned();
        tell(
            &mut opened,
            (&terminal, Event::OpenFile { path, line: None }),
        );
        let editors = opened.shell.update(&mut opened.window, |shell, _| {
            let editors = panes_of(shell, EDITOR);
            let layout = shell.state.layout();
            editors
                .iter()
                .filter_map(|pane| layout.pane(pane).map(|i| i.state.as_value().clone()))
                .collect::<Vec<_>>()
        });
        assert_eq!(
            editors,
            [serde_json::json!({ "path": "/etc/hosts", "absolute": true })]
        );
    }

    #[gpui::test]
    fn a_home_path_opens_nothing(cx: &mut TestAppContext) {
        let (mut opened, terminal) = with_terminal(cx);
        let path = "~/foo/bar".to_owned();
        tell(
            &mut opened,
            (&terminal, Event::OpenFile { path, line: None }),
        );
        let editors = opened
            .shell
            .update(&mut opened.window, |shell, _| panes_of(shell, EDITOR));
        assert!(editors.is_empty(), "{editors:?}");
    }
}
