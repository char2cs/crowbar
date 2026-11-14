use protocol::{PaneId, PaneTypeId, Params};
use serde_json::json;

use super::*;

fn runtime() -> PaneRuntime {
    PaneRuntime::new(PaneInstance {
        pane_id: PaneId::new("p1").unwrap(),
        type_id: PaneTypeId::new("core.chat").unwrap(),
        params: Params::default().with("threadId", "t"),
        state: PaneState::new(json!({"scroll": 3})).unwrap(),
    })
}

#[test]
fn mount_carries_params_and_state() {
    let frame = runtime().lifecycle_frame(Lifecycle::Mount);
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

#[test]
fn a_discarded_state_is_mounted_empty_but_other_events_keep_it() {
    let mut runtime = runtime();
    runtime.discard_state = true;
    let ShellFrame::Lifecycle { state, .. } = runtime.lifecycle_frame(Lifecycle::Mount) else {
        panic!("not a lifecycle frame");
    };
    assert_eq!(state, PaneState::default(), "mount without state");
    let ShellFrame::Lifecycle { state, .. } = runtime.lifecycle_frame(Lifecycle::Destroy) else {
        panic!("not a lifecycle frame");
    };
    assert_ne!(state, PaneState::default(), "destroy still carries it");
}

#[test]
fn detach_returns_the_pane_to_parked() {
    let mut runtime = runtime();
    runtime.phase = PanePhase::Mounted;
    runtime.slot = Some(SlotId(1));
    runtime.visible = true;
    runtime.activated = true;
    assert!(runtime.is_mounted(), "mounted");
    runtime.detach();
    assert_eq!(
        (
            runtime.slot,
            runtime.visible,
            runtime.activated,
            runtime.phase.clone()
        ),
        (None, false, false, PanePhase::Parked),
        "reset"
    );
}
