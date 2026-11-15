use model::PaneInstance;
use protocol::{Lifecycle, PaneId, PaneState, PaneTypeId, ShellFrame};

use crate::pane_phase::PanePhase;
use crate::slot_id::SlotId;
use crate::stage::Stage;

#[derive(Debug)]
pub(crate) struct PaneRuntime {
    pub(crate) type_id: PaneTypeId,
    pub(crate) phase: PanePhase,
    pub(crate) slot: Option<SlotId>,
    pub(crate) stage: Stage,
    pub(crate) discard_state: bool,
}

impl PaneRuntime {
    pub(crate) fn new(type_id: PaneTypeId) -> Self {
        Self {
            type_id,
            phase: PanePhase::Parked,
            slot: None,
            stage: Stage::Hidden,
            discard_state: false,
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

    fn pane() -> PaneId {
        PaneId::new("p1").unwrap()
    }

    fn instance() -> PaneInstance {
        PaneInstance {
            pane_id: pane(),
            type_id: PaneTypeId::new("core.chat").unwrap(),
            params: Params::default().with("threadId", "t").unwrap(),
            state: PaneState::new(json!({"scroll": 3})).unwrap(),
        }
    }

    fn runtime() -> PaneRuntime {
        PaneRuntime::new(PaneTypeId::new("core.chat").unwrap())
    }

    fn frame_state(
        runtime: &PaneRuntime,
        event: Lifecycle,
    ) -> PaneState {
        let ShellFrame::Lifecycle { state, .. } =
            runtime.lifecycle_frame(&pane(), event, Some(&instance()))
        else {
            panic!("not a lifecycle frame");
        };
        state
    }

    #[test]
    fn mount_carries_params_and_state_from_the_model() {
        let frame = runtime().lifecycle_frame(&pane(), Lifecycle::Mount, Some(&instance()));
        let ShellFrame::Lifecycle {
            pane_id,
            event,
            params,
            state,
        } = frame
        else {
            panic!("not a lifecycle frame");
        };
        assert_eq!(
            (pane_id.as_str(), event),
            ("p1", Lifecycle::Mount),
            "identity"
        );
        assert_eq!(params.get("threadId"), Some("t"), "params");
        assert_eq!(state.as_value(), &json!({"scroll": 3}), "state");
    }

    type Case = (&'static str, Lifecycle, bool, bool);

    const CASES: &[Case] = &[
        (
            "mount carries the model state",
            Lifecycle::Mount,
            false,
            true,
        ),
        (
            "mount after a discard is empty",
            Lifecycle::Mount,
            true,
            false,
        ),
        (
            "destroy keeps the state after a discard",
            Lifecycle::Destroy,
            true,
            true,
        ),
        ("park keeps the state", Lifecycle::Park, false, true),
    ];

    #[test]
    fn a_discarded_state_is_mounted_empty_but_other_events_keep_it() {
        for (name, event, discard, keeps_state) in CASES {
            let mut runtime = runtime();
            runtime.discard_state = *discard;
            let carried = frame_state(&runtime, *event) != PaneState::default();
            assert_eq!(carried, *keeps_state, "{name}");
        }
    }

    #[test]
    fn a_pane_the_model_no_longer_has_gets_an_empty_frame() {
        let frame = runtime().lifecycle_frame(&pane(), Lifecycle::Destroy, None);
        let ShellFrame::Lifecycle { params, state, .. } = frame else {
            panic!("not a lifecycle frame");
        };
        assert_eq!(
            (params, state),
            (Params::default(), PaneState::default()),
            "empty"
        );
    }

    #[test]
    fn detach_returns_the_pane_to_parked() {
        let mut runtime = runtime();
        runtime.phase = PanePhase::Mounted;
        runtime.slot = Some(SlotId(1));
        runtime.stage = Stage::Active;
        assert!(runtime.is_mounted(), "mounted");
        runtime.detach();
        assert_eq!(
            (runtime.slot, runtime.stage, runtime.phase.clone()),
            (None, Stage::Hidden, PanePhase::Parked),
            "reset"
        );
    }
}
