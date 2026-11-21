use layout::Command;
use protocol::{DocumentPath, PaneId};

use super::ShellState;
use crate::editor_panes::EditorTarget;
use crate::pane_spec::PaneSpec;

impl ShellState {
    pub(crate) fn settle_file_pane(
        &mut self,
        pane: &PaneId,
    ) {
        let instance = self.history.layout().pane(pane);
        let Some(state) = instance.and_then(|instance| EditorTarget::settled(&instance.state))
        else {
            return;
        };
        self.run(vec![Command::SetPaneState {
            pane: pane.clone(),
            state,
        }]);
    }

    pub(crate) fn file_pane(
        &self,
        (chat, path): (&str, &DocumentPath),
    ) -> Option<PaneId> {
        let spec = PaneSpec::editor(chat, path.as_str()).ok()?;
        let found = self.history.layout().find_pane(&spec.type_id, &spec.params);
        found.map(|instance| instance.pane_id.clone())
    }

    pub(crate) fn replace_untitled(
        &mut self,
        untitled: &PaneId,
        (chat, path): (&str, &DocumentPath),
    ) -> Option<PaneId> {
        let spec = PaneSpec::editor(chat, path.as_str()).ok()?;
        let shown = match self.file_pane((chat, path)) {
            Some(open) => open,
            None => self.fresh_pane()?,
        };
        let index = self.strip_index(untitled);
        self.dirty.clear(untitled);
        self.apply(vec![
            Command::OpenPaneInGroup {
                instance: spec.instance(shown.clone()),
                onto: untitled.clone(),
                index,
            },
            Command::ClosePane {
                pane: untitled.clone(),
            },
        ]);
        self.settle_file_pane(&shown);
        Some(shown)
    }

    fn fresh_pane(&self) -> Option<PaneId> {
        match PaneId::new(self.serials.next_pane()) {
            Ok(pane) => Some(pane),
            Err(error) => {
                tracing::warn!(%error, "no pane id for the saved file");
                None
            },
        }
    }

    fn strip_index(
        &self,
        pane: &PaneId,
    ) -> Option<usize> {
        let layout = self.history.layout();
        let group = layout.tabs().iter().find_map(|tab| tab.group_of(pane))?;
        group.tabs().iter().position(|member| member == pane)
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::test_support::shell_rig;

    #[test]
    fn a_file_that_showed_is_restored_as_kept_not_opened_afresh() {
        let mut rig = shell_rig();
        rig.shell
            .open_spec(PaneSpec::editor("c1", "src/a.ts").unwrap());
        let pane = rig.shell.layout().tabs()[0].pane_ids()[0].clone();
        rig.shell.settle_file_pane(&pane);
        let state = rig.shell.layout().pane(&pane).unwrap().state.clone();
        assert_eq!(state.as_value(), &json!({"path": "src/a.ts"}));
    }

    fn strip(rig: &crate::test_support::ShellRig) -> Vec<String> {
        let layout = rig.shell.layout();
        let panes = layout.tabs()[0].pane_ids();
        panes
            .iter()
            .filter_map(|pane| layout.pane(pane))
            .map(|instance| instance.state.as_value().to_string())
            .collect()
    }

    #[test]
    fn an_untitled_tab_saved_as_a_file_becomes_that_file_in_its_place() {
        let untitled = r#"{"untitled":true}"#;
        let cases = [
            (
                "a new file",
                "src/new.ts",
                vec![r#"{"path":"src/new.ts"}"#, r#"{"path":"src/b.ts"}"#],
            ),
            (
                "an open file moves into its place",
                "src/b.ts",
                vec![r#"{"path":"src/b.ts"}"#],
            ),
        ];
        for (name, target, expected) in cases {
            let mut rig = shell_rig();
            rig.shell.open_spec(PaneSpec::untitled("c1", "p4").unwrap());
            rig.shell
                .open_spec(PaneSpec::editor("c1", "src/b.ts").unwrap());
            let pane = rig.shell.layout().tabs()[0].pane_ids()[0].clone();
            let b = rig.shell.layout().tabs()[0].pane_ids()[1].clone();
            rig.shell.settle_file_pane(&b);
            assert_eq!(strip(&rig)[0], untitled, "{name}: untitled first");
            let path = DocumentPath::new(target).unwrap();
            let shown = rig.shell.replace_untitled(&pane, ("c1", &path));
            assert_eq!(
                shown,
                rig.shell.file_pane(("c1", &path)),
                "{name}: shows the file"
            );
            assert_eq!(strip(&rig), expected, "{name}");
        }
    }
}
