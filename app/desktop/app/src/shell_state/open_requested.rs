use layout::Command;
use protocol::{DocumentPath, OpenMode, PaneId, PaneOpen};

use super::ShellState;
use super::preview::previewing;
use crate::core_panes::EDITOR;
use crate::error::Result;
use crate::focused_chat::chat_of_pane;
use crate::pane_opener::open_commands;
use crate::pane_spec::PaneSpec;

impl ShellState {
    pub(crate) fn open_requested(
        &mut self,
        from: &PaneId,
        open: PaneOpen,
    ) {
        let document = open.file.filter(|_| open.type_id.as_str() == EDITOR);
        let commands = match document {
            Some(file) => self.document_commands(from, &file),
            None => open_commands(
                self.history.layout(),
                (open.type_id, open.params),
                &self.serials,
            ),
        };
        match commands {
            Ok(commands) => self.apply(kept_or_previewed(commands, open.mode)),
            Err(error) => tracing::warn!(%error, "could not open the pane"),
        }
    }

    fn document_commands(
        &self,
        from: &PaneId,
        file: &DocumentPath,
    ) -> Result<Vec<Command>> {
        let layout = self.history.layout();
        let Some(chat) = chat_of_pane(layout, from) else {
            tracing::warn!(%from, "a pane with no chat asked to open a document");
            return Ok(Vec::new());
        };
        let path = file.as_str();
        if file.is_host() {
            return PaneSpec::host_file(&chat, path)?.open_commands(layout, &self.serials);
        }
        PaneSpec::editor(&chat, path)?.open_commands(layout, &self.serials)
    }
}

fn kept_or_previewed(
    commands: Vec<Command>,
    mode: OpenMode,
) -> Vec<Command> {
    match mode {
        OpenMode::Kept => commands,
        OpenMode::Preview => previewing(commands),
    }
}

#[cfg(test)]
mod tests {
    use panehost::IpcMessage;
    use protocol::{PaneFrame, PaneTypeId, Params, encode};

    use super::*;
    use crate::test_support::{open_chat, resync, shell_rig};

    fn opened_from_the_chat(
        file: &str,
        mode: OpenMode,
    ) -> (Option<String>, Option<serde_json::Value>) {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "t1");
        resync(&mut rig);
        let tab = rig.shell.layout().active_tab().unwrap();
        let chat = tab.pane_ids().into_iter().next().unwrap();
        let frame = PaneFrame::Open(PaneOpen {
            type_id: PaneTypeId::new(EDITOR).unwrap(),
            params: Params::default(),
            file: Some(DocumentPath::new(file).unwrap()),
            mode,
        });
        let text = encode(&frame).unwrap();
        rig.shell.handle_ipc(IpcMessage {
            pane: chat.clone(),
            text,
        });
        let tab = rig.shell.layout().active_tab().unwrap();
        let group = tab.group_of(&chat).unwrap();
        let opened = group
            .tabs()
            .last()
            .and_then(|pane| rig.shell.layout().pane(pane));
        let preview = group.preview().map(ToString::to_string);
        (
            preview,
            opened.map(|instance| instance.state.as_value().clone()),
        )
    }

    #[test]
    fn a_page_opens_a_workspace_document_as_a_tab_of_its_chat_kept_or_previewed() {
        for mode in [OpenMode::Kept, OpenMode::Preview] {
            let (preview, state) = opened_from_the_chat("src/a.ts", mode);
            let state = state.expect("an editor tab");
            assert_eq!(state["path"], "src/a.ts", "{mode:?}");
            assert_eq!(preview.is_some(), mode == OpenMode::Preview, "{mode:?}");
        }
    }

    #[test]
    fn a_page_opens_a_host_document_by_its_absolute_path() {
        let (_, state) = opened_from_the_chat("/etc/hosts", OpenMode::Kept);
        let state = state.expect("an editor tab");
        assert_eq!(
            (&state["path"], &state["absolute"]),
            (&"/etc/hosts".into(), &true.into())
        );
    }
}
