//! Showing a changed file in a review pane that is already open.

use layout::Command;
use protocol::{PaneId, PaneState, ShellFrame};
use serde_json::json;

use crate::shell_state::ShellState;

fn with_reveal(
    saved: &PaneState,
    path: &str,
) -> Option<PaneState> {
    let mut value = saved.as_value().clone();
    if !value.is_object() {
        value = json!({});
    }
    let fields = value.as_object_mut()?;
    fields.insert("reveal".into(), json!(path));
    PaneState::new(value).ok()
}

impl ShellState {
    fn tell_review(
        &self,
        pane: &PaneId,
        path: &str,
    ) {
        let frame = ShellFrame::ReviewReveal {
            path: path.to_owned(),
        };
        if let Err(error) = self.host().send(pane, &frame) {
            tracing::warn!(%error, "the review could not be told what to show");
        }
    }

    pub(crate) fn reveal_in_review(
        &mut self,
        pane: &PaneId,
        path: &str,
    ) {
        if self.host().is_mounted(pane) {
            self.tell_review(pane, path);
            return;
        }
        let saved = self
            .layout()
            .pane(pane)
            .and_then(|p| with_reveal(&p.state, path));
        let Some(state) = saved else {
            return;
        };
        self.apply(vec![Command::SetPaneState {
            pane: pane.clone(),
            state,
        }]);
    }
}

#[cfg(test)]
mod tests {
    use crate::pane_spec::PaneSpec;
    use crate::test_support::shell_rig;

    #[test]
    fn a_review_that_has_not_mounted_keeps_the_reveal_for_its_mount() {
        let mut rig = shell_rig();
        rig.shell.open_spec(PaneSpec::review("c1", None).unwrap());
        let pane = rig.shell.layout().showing("").unwrap().pane_ids()[0].clone();
        rig.shell.reveal_in_review(&pane, "src/a.ts");
        let saved = rig
            .shell
            .layout()
            .pane(&pane)
            .map(|p| p.state.as_value().clone());
        assert_eq!(saved, Some(serde_json::json!({ "reveal": "src/a.ts" })));
    }
}
