use layout::Command;
use protocol::PaneId;

use super::ShellState;
use crate::pane_spec::PaneSpec;

impl ShellState {
    pub(crate) fn preview_spec(
        &mut self,
        spec: PaneSpec,
    ) {
        match spec.open_commands(self.history.layout(), &self.serials) {
            Ok(commands) => self.apply(previewing(commands)),
            Err(error) => tracing::warn!(%error, "could not preview the pane"),
        }
    }

    pub(super) fn keep_if_preview(
        &mut self,
        pane: &PaneId,
    ) {
        let layout = self.history.layout();
        let group = layout.tab_of(pane).and_then(|tab| tab.group_of(pane));
        if group.and_then(layout::PaneGroup::preview) != Some(pane) {
            return;
        }
        self.run(vec![Command::KeepPane { pane: pane.clone() }]);
    }
}

fn previewing(commands: Vec<Command>) -> Vec<Command> {
    let fresh: Vec<PaneId> = commands.iter().filter_map(opened_pane).collect();
    let previews = fresh.into_iter().map(|pane| Command::PreviewPane { pane });
    commands.into_iter().chain(previews).collect()
}

fn opened_pane(command: &Command) -> Option<PaneId> {
    match command {
        Command::OpenPane { instance, .. } | Command::OpenPaneInGroup { instance, .. } => {
            Some(instance.pane_id.clone())
        },
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use panehost::IpcMessage;
    use protocol::{PaneFrame, PaneState, encode};

    use super::*;
    use crate::test_support::{open_chat, resync, shell_rig};

    fn preview_of(shell: &ShellState) -> Option<String> {
        let tab = shell.layout().active_tab()?;
        let chat = tab.pane_ids().into_iter().next()?;
        let preview = tab.group_of(&chat)?.preview()?;
        let state: &PaneState = &shell.layout().pane(preview)?.state;
        let state = state.as_value();
        state.get("path")?.as_str().map(ToOwned::to_owned)
    }

    #[test]
    fn a_file_opened_from_the_tree_is_the_preview_until_another_one_is() {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "t1");
        let steps = [("a.rs", "a.rs"), ("b.rs", "b.rs"), ("a.rs", "b.rs")];
        for (opened, expected) in steps {
            rig.shell
                .preview_spec(PaneSpec::editor("t1", opened).unwrap());
            let preview = preview_of(&rig.shell);
            assert_eq!(preview.as_deref(), Some(expected), "after {opened}");
        }
    }

    #[test]
    fn a_first_edit_keeps_the_preview_as_an_ordinary_tab() {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "t1");
        rig.shell
            .preview_spec(PaneSpec::editor("t1", "a.rs").unwrap());
        resync(&mut rig);
        let tab = rig.shell.layout().active_tab().unwrap();
        let pane = tab.pane_ids().into_iter().last().unwrap();
        let text = encode(&PaneFrame::SetDirty { dirty: true }).unwrap();
        rig.shell.handle_ipc(IpcMessage { pane, text });
        assert_eq!(preview_of(&rig.shell), None);
    }
}
