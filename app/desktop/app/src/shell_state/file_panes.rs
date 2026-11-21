use layout::Command;
use protocol::PaneId;

use super::ShellState;
use crate::editor_panes::EditorTarget;

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
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use crate::pane_spec::PaneSpec;
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
}
