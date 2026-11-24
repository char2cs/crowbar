use layout::Command;
use protocol::{DocumentPath, OpenMode, PaneId, PaneOpen};

use super::ShellState;
use super::preview::{keeping, previewing};
use crate::core_panes::EDITOR;
use crate::error::Result;
use crate::focused_chat::chat_of_pane;
use crate::pane_opener::open_commands;
use crate::pane_spec::PaneSpec;

impl ShellState {
    pub(crate) fn requested_open(
        &self,
        from: &PaneId,
        open: PaneOpen,
    ) -> Vec<Command> {
        let document = open.file.filter(|_| open.type_id.as_str() == EDITOR);
        let commands = match document {
            Some(file) => self.document_commands(from, &file),
            None => open_commands(
                self.history.layout(),
                &self.space.active(),
                (open.type_id, open.params),
                &self.serials,
            ),
        };
        match commands {
            Ok(commands) => kept_or_previewed(self.history.layout(), commands, open.mode),
            Err(error) => {
                tracing::warn!(%error, "could not open the pane");
                Vec::new()
            },
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
            return self.spec_commands(PaneSpec::host_file(&chat, path)?);
        }
        self.spec_commands(PaneSpec::editor(&chat, path)?)
    }
}

fn kept_or_previewed(
    layout: &layout::Layout,
    commands: Vec<Command>,
    mode: OpenMode,
) -> Vec<Command> {
    match mode {
        OpenMode::Kept => keeping(layout, commands),
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
        let tab = rig.shell.showing().unwrap();
        let chat = tab.pane_ids().into_iter().next().unwrap();
        let frame = PaneFrame::Open(PaneOpen {
            type_id: PaneTypeId::new(EDITOR).unwrap(),
            params: Params::default(),
            file: Some(DocumentPath::new(file).unwrap()),
            mode,
        });
        let text = encode(&frame).unwrap();
        let outcome = rig.shell.handle_ipc(IpcMessage {
            pane: chat.clone(),
            text,
        });
        rig.shell.apply(outcome.opens);
        let tab = rig.shell.showing().unwrap();
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

    fn shown_after_the_chat_opens((file, mode): (&str, OpenMode)) -> Option<serde_json::Value> {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "t1");
        resync(&mut rig);
        let tab = rig.shell.showing().unwrap();
        let chat = tab.pane_ids().into_iter().next().unwrap();
        for kept in ["src/a.ts", "src/pricing.ts"] {
            rig.shell.open_spec(PaneSpec::editor("t1", kept).unwrap());
        }
        resync(&mut rig);
        let frame = PaneFrame::Open(PaneOpen {
            type_id: PaneTypeId::new(EDITOR).unwrap(),
            params: Params::default(),
            file: Some(DocumentPath::new(file).unwrap()),
            mode,
        });
        let text = encode(&frame).unwrap();
        let outcome = rig.shell.handle_ipc(IpcMessage { pane: chat, text });
        rig.shell.apply(outcome.opens);
        let layout = rig.shell.layout();
        let editor = layout.find_pane(&PaneTypeId::new(EDITOR).unwrap(), &file_params(file))?;
        let group = layout.tab_of(&editor.pane_id)?.group_of(&editor.pane_id)?;
        let shown = layout.pane(group.shown()?)?;
        Some(shown.state.as_value()["path"].clone())
    }

    fn file_params(file: &str) -> Params {
        let spec = PaneSpec::editor("t1", file).unwrap();
        spec.instance(PaneId::new("probe").unwrap()).params
    }

    const CASES: [(&str, &str, OpenMode); 4] = [
        ("a new file, previewed", "src/b.ts", OpenMode::Preview),
        ("a new file, kept", "src/b.ts", OpenMode::Kept),
        ("an open file, previewed", "src/a.ts", OpenMode::Preview),
        ("an open file, kept", "src/a.ts", OpenMode::Kept),
    ];

    #[test]
    fn a_page_shows_the_file_it_opens_whether_new_or_already_open() {
        for (name, file, mode) in CASES {
            assert_eq!(
                shown_after_the_chat_opens((file, mode)),
                Some(file.into()),
                "{name}"
            );
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
