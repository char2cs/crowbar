use layout::Command;
use panehost::HostEvent;
use protocol::PaneState;

pub(crate) fn commands_for(events: &[HostEvent]) -> Vec<Command> {
    events
        .iter()
        .filter_map(|event| match event {
            HostEvent::StateChanged { pane, state } => Some(Command::SetPaneState {
                pane: pane.clone(),
                state: state.clone(),
            }),
            HostEvent::ParamsChanged { pane, params } => Some(Command::SetPaneParams {
                pane: pane.clone(),
                params: params.clone(),
            }),
            HostEvent::StateDiscarded { pane } => Some(Command::SetPaneState {
                pane: pane.clone(),
                state: PaneState::default(),
            }),
            _ => None,
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use protocol::PaneId;
    use serde_json::json;

    use super::*;

    #[test]
    fn state_events_become_set_state_commands() {
        let pane = PaneId::new("p").unwrap();
        let state = PaneState::new(json!({"k": 1})).unwrap();
        let events = [
            HostEvent::StateChanged {
                pane: pane.clone(),
                state: state.clone(),
            },
            HostEvent::StateDiscarded { pane: pane.clone() },
            HostEvent::TitleChanged {
                pane: pane.clone(),
                title: protocol::PaneTitle::new("t").unwrap(),
            },
        ];
        assert_eq!(
            commands_for(&events),
            [
                Command::SetPaneState {
                    pane: pane.clone(),
                    state
                },
                Command::SetPaneState {
                    pane,
                    state: PaneState::default()
                },
            ],
            "only state events map to commands"
        );
    }

    #[test]
    fn a_params_event_re_keys_its_pane() {
        let pane = PaneId::new("p").unwrap();
        let params = protocol::Params::default().with("threadId", "c1").unwrap();
        let events = [HostEvent::ParamsChanged {
            pane: pane.clone(),
            params: params.clone(),
        }];
        assert_eq!(
            commands_for(&events),
            [Command::SetPaneParams { pane, params }]
        );
    }
}
