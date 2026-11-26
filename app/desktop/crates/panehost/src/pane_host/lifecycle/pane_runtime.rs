use layout::PaneInstance;
use protocol::{Lifecycle, PaneId, PaneState, PaneTypeId, ShellFrame};

use crate::catalog::PaneKind;
use crate::pane_host::lifecycle::pane_phase::PanePhase;
use crate::pane_host::lifecycle::stage::Stage;
use crate::slot::Id;

#[derive(Debug)]
pub(crate) struct PaneRuntime {
    pub(crate) type_id: PaneTypeId,
    pub(crate) kind: PaneKind,
    pub(crate) phase: PanePhase,
    pub(crate) slot: Option<Id>,
    pub(crate) stage: Stage,
    pub(crate) covered: bool,
    pub(crate) discard_state: bool,
    pub(crate) violations: u32,
    pub(crate) chat_fills: Option<bool>,
}

impl PaneRuntime {
    pub(crate) fn new(type_id: PaneTypeId) -> Self {
        Self {
            type_id,
            kind: PaneKind::Web,
            phase: PanePhase::Parked,
            slot: None,
            stage: Stage::Hidden,
            covered: false,
            discard_state: false,
            violations: 0,
            chat_fills: None,
        }
    }

    pub(crate) fn lifecycle_frame(
        &self,
        pane: &PaneId,
        event: Lifecycle,
        instance: Option<&PaneInstance>,
    ) -> ShellFrame {
        let params = instance.map_or_else(Default::default, |instance| instance.params.clone());
        let state = match (event, self.discard_state, instance) {
            (Lifecycle::Mount, true, _) | (_, _, None) => PaneState::default(),
            (_, _, Some(instance)) => instance.state.clone(),
        };
        ShellFrame::Lifecycle {
            pane_id: pane.clone(),
            event,
            params,
            state,
        }
    }

    pub(crate) fn reveal(&mut self) {
        if self.stage == Stage::Hidden {
            self.stage = Stage::Shown;
        }
    }

    pub(crate) fn is_visible(&self) -> bool {
        self.stage != Stage::Hidden
    }

    pub(crate) fn is_mounted(&self) -> bool {
        self.phase == PanePhase::Mounted
    }

    pub(crate) fn detach(&mut self) {
        self.slot = None;
        self.stage = Stage::Hidden;
        self.phase = PanePhase::Parked;
    }
}

#[cfg(test)]
mod tests {
    use protocol::Params;
    use serde_json::json;

    use super::*;

    fn kept(
        event: Lifecycle,
        instance: Option<&PaneInstance>,
    ) -> (bool, bool) {
        let mut runtime = PaneRuntime::new(PaneTypeId::new("core.chat").unwrap());
        runtime.discard_state = true;
        let pane = PaneId::new("p1").unwrap();
        match runtime.lifecycle_frame(&pane, event, instance) {
            ShellFrame::Lifecycle { state, params, .. } => {
                (state != PaneState::default(), params != Params::default())
            },
            other => panic!("not a lifecycle frame: {other:?}"),
        }
    }

    #[test]
    fn a_discarded_state_is_mounted_empty_but_other_events_keep_it_and_gone_panes_send_none() {
        let instance = PaneInstance {
            pane_id: PaneId::new("p1").unwrap(),
            type_id: PaneTypeId::new("core.chat").unwrap(),
            params: Params::default().with("threadId", "t").unwrap(),
            state: PaneState::new(json!({"scroll": 3})).unwrap(),
        };
        let cases = [
            (
                "mount after a discard",
                Lifecycle::Mount,
                Some(&instance),
                (false, true),
            ),
            (
                "destroy after a discard",
                Lifecycle::Destroy,
                Some(&instance),
                (true, true),
            ),
            (
                "park after a discard",
                Lifecycle::Park,
                Some(&instance),
                (true, true),
            ),
            (
                "a pane the model no longer has",
                Lifecycle::Destroy,
                None,
                (false, false),
            ),
        ];
        for (name, event, instance, expected) in cases {
            assert_eq!(kept(event, instance), expected, "{name}");
        }
    }
}
