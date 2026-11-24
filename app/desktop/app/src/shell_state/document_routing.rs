use layout::Command;
use protocol::PaneId;

use super::ShellState;
use crate::editor_panes::EditorTarget;
use crate::error::Result;
use crate::pane_opener::focus_commands;
use crate::pane_spec::PaneSpec;

impl ShellState {
    pub(super) fn spec_commands(
        &self,
        spec: PaneSpec,
    ) -> Result<Vec<Command>> {
        let layout = self.history.layout();
        if let Some(holder) = self.document_holder(&spec) {
            return Ok(focus_commands(layout, &holder));
        }
        spec.open_commands((layout, &self.space.active()), &self.serials)
    }

    pub(crate) fn document_pane(
        &self,
        spec: &PaneSpec,
    ) -> Option<PaneId> {
        let exact = || self.history.layout().find_pane(&spec.type_id, &spec.params);
        let exact = || exact().map(|instance| instance.pane_id.clone());
        self.document_holder(spec).or_else(exact)
    }

    fn document_holder(
        &self,
        spec: &PaneSpec,
    ) -> Option<PaneId> {
        let wanted = EditorTarget::of_pane((&spec.type_id, &spec.params, &spec.state))?;
        let layout = self.history.layout();
        let tab = layout.showing(&self.space.active())?;
        let holds = |pane: &PaneId| {
            let held = layout.pane(pane).and_then(EditorTarget::of);
            held.is_some_and(|held| self.same_document(&held, &wanted))
        };
        tab.pane_ids().into_iter().find(holds)
    }

    fn same_document(
        &self,
        held: &EditorTarget,
        wanted: &EditorTarget,
    ) -> bool {
        let shown = held.preview.is_none() && wanted.preview.is_none();
        let Some(file) = wanted.file.as_ref().filter(|_| shown) else {
            return false;
        };
        if held.file.as_ref() != Some(file) {
            return false;
        }
        if file.is_host() || held.chat == wanted.chat {
            return true;
        }
        let workspace = |chat: &str| self.space.workspace_of_chat(chat);
        workspace(&held.chat).is_some_and(|own| workspace(&wanted.chat) == Some(own))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::shell_rig;

    type Routed = (&'static str, PaneSpec, PaneSpec, bool);

    fn routing_cases() -> [Routed; 4] {
        let editor = |chat, path| PaneSpec::editor(chat, path).unwrap();
        let host = |chat| PaneSpec::host_file(chat, "/etc/hosts").unwrap();
        [
            (
                "another chat of its workspace",
                editor("c1", "src/a.ts"),
                editor("c2", "src/a.ts"),
                true,
            ),
            (
                "a chat of another workspace",
                editor("c1", "src/a.ts"),
                editor("c3", "src/a.ts"),
                false,
            ),
            (
                "another file of its workspace",
                editor("c1", "src/a.ts"),
                editor("c2", "src/b.ts"),
                false,
            ),
            ("a host file from any chat", host("c1"), host("c3"), true),
        ]
    }

    #[test]
    fn opening_a_file_open_under_another_chat_of_its_workspace_focuses_that_pane() {
        for (name, held, asked, focused) in routing_cases() {
            let mut rig = shell_rig();
            rig.space.know_workspace(("c1", "w1"));
            rig.space.know_workspace(("c2", "w1"));
            rig.space.know_workspace(("c3", "w2"));
            rig.shell.open_spec(held);
            let holder = rig.shell.layout().tabs()[0].pane_ids()[0].clone();
            rig.shell
                .open_spec(PaneSpec::terminal("t1", Some("c1"), None).unwrap());
            rig.shell.open_spec(asked);
            let shown = rig.shell.showing().and_then(layout::Tab::focused).cloned();
            assert_eq!(
                shown == Some(holder),
                focused,
                "{name}: the holder is focused"
            );
            let expected = 3 - usize::from(focused);
            assert_eq!(rig.shell.layout().pane_count(), expected, "{name}: panes");
        }
    }
}
