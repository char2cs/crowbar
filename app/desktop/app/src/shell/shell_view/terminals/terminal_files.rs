use gpui::Context;
use layout::Command;
use protocol::PaneId;
use ui::{ToastContent, ToastKind};

use crate::panes::kinds::pane_spec::PaneSpec;
use crate::panes::terminals::file_target::FileTarget;
use crate::panes::terminals::terminal_owner::TerminalOwner;
use crate::panes::terminals::terminal_root;
use crate::shell::shell_view::ShellView;
use crate::shell::window::notice_key::NoticeKey;

const CANNOT_OPEN: &str = "Cannot open file";
const LINE: &str = "line";

impl ShellView {
    pub(crate) fn terminal_chat(
        &self,
        pane: &PaneId,
    ) -> Option<String> {
        match self.state.terminals().owner_of(pane)? {
            TerminalOwner::Chat(chat) => Some(chat),
            TerminalOwner::Home(_) => None,
        }
    }

    pub(crate) fn open_printed_file(
        &mut self,
        pane: &PaneId,
        (printed, line): (&str, Option<u32>),
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
        let root = terminal_root::terminal_root(directory, workspace.as_deref());
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
        self.open_target(chat, (target, line), cx);
    }

    fn open_target(
        &mut self,
        chat: Option<String>,
        (target, line): (FileTarget, Option<u32>),
        cx: &mut Context<'_, Self>,
    ) {
        let chat = chat.or_else(|| self.chat_for("a file"));
        let Some(chat) = chat else {
            return;
        };
        match target {
            FileTarget::Workspace(relative) => {
                self.open_spec(at_line(PaneSpec::editor(&chat, &relative), line), cx);
            },
            FileTarget::Host(absolute) => {
                self.open_spec(at_line(PaneSpec::host_file(&chat, &absolute), line), cx);
            },
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

pub(crate) fn at_line(
    spec: crate::error::Result<PaneSpec>,
    line: Option<u32>,
) -> crate::error::Result<PaneSpec> {
    let spec = spec?;
    let Some(line) = line else {
        return Ok(spec);
    };
    let mut fields = spec.state.as_value().clone();
    let Some(object) = fields.as_object_mut() else {
        return Ok(spec);
    };
    object.insert(LINE.to_owned(), serde_json::Value::from(line));
    let state = protocol::PaneState::new(fields)?;
    Ok(PaneSpec { state, ..spec })
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use terminalview::Event;

    use crate::panes::kinds::core_panes::EDITOR;
    use crate::shell::shell_view::test_support::{panes_of, tell, with_terminal};

    #[gpui::test]
    fn an_absolute_path_opens_as_a_file_of_this_computer_at_its_line(cx: &mut TestAppContext) {
        let (mut opened, terminal) = with_terminal(cx);
        let path = "/etc/hosts".to_owned();
        tell(
            &mut opened,
            (
                &terminal,
                Event::OpenFile {
                    path,
                    line: Some(3),
                },
            ),
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
            [
                serde_json::json!({ "path": "/etc/hosts", "absolute": true, "fresh": true, "line": 3 })
            ]
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
