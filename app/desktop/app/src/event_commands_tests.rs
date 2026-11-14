use protocol::PaneId;
use serde_json::json;

use super::*;

#[test]
fn state_events_become_set_state_commands() {
    let pane = PaneId::new("p").unwrap();
    let state = PaneState::new(json!({"k": 1})).unwrap();
    let events = [
        HostEvent::StateChanged { pane: pane.clone(), state: state.clone() },
        HostEvent::StateDiscarded { pane: pane.clone() },
        HostEvent::TitleChanged { pane: pane.clone(), title: "t".into() },
    ];
    assert_eq!(
        commands_for(&events),
        [
            Command::SetPaneState { pane: pane.clone(), state },
            Command::SetPaneState { pane, state: PaneState::default() },
        ],
        "only state events map to commands"
    );
}
