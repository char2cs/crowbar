use layout::Command;
use protocol::{PaneId, PaneState};
use serde_json::{Map, Value};

use super::ShellState;
use crate::panes::terminals::terminal_ids::NAME_KEY;

impl ShellState {
    pub(crate) fn rename_pane(
        &mut self,
        pane: &PaneId,
        name: &str,
    ) {
        let Some(instance) = self.history.layout().pane(pane) else {
            return;
        };
        let Some(state) = named_state(&instance.state, name) else {
            return;
        };
        self.titles.retain(|titled| titled != pane);
        self.apply(vec![Command::SetPaneState {
            pane: pane.clone(),
            state,
        }]);
        self.terminals.name_into(&self.titles);
    }
}

fn named_state(
    state: &PaneState,
    name: &str,
) -> Option<PaneState> {
    let mut fields: Map<String, Value> = state.as_value().as_object().cloned().unwrap_or_default();
    fields.insert(NAME_KEY.to_owned(), Value::from(name));
    PaneState::new(Value::Object(fields)).ok()
}

#[cfg(test)]
mod tests {
    use crate::panes::kinds::pane_spec::PaneSpec;
    use crate::test_support::shell_rig;

    #[test]
    fn renaming_a_terminal_names_its_tab_and_keeps_its_folder() {
        let mut rig = shell_rig();
        let spec = PaneSpec::terminal("t1", None, Some("/work/src")).unwrap();
        rig.shell.open_spec(spec);
        let pane = rig.shell.layout().tabs()[0].pane_ids()[0].clone();
        rig.shell.rename_pane(&pane, "old");
        rig.shell.rename_pane(&pane, "logs");
        let title = rig.shell.titles().get(&pane).map(|t| t.to_string());
        assert_eq!(title.as_deref(), Some("logs"));
        let state = rig.shell.layout().pane(&pane).unwrap().state.clone();
        assert_eq!(state.as_value()["directory"], "/work/src", "folder");
    }

    #[test]
    fn the_name_survives_in_the_saved_layout() {
        let mut rig = shell_rig();
        let spec = PaneSpec::terminal("t1", None, None).unwrap();
        rig.shell.open_spec(spec);
        let pane = rig.shell.layout().tabs()[0].pane_ids()[0].clone();
        rig.shell.rename_pane(&pane, "logs");
        let state = rig.shell.layout().pane(&pane).unwrap().state.clone();
        assert_eq!(state.as_value()["title"], "logs");
    }
}
