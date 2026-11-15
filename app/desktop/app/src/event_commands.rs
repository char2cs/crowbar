use model::Command;
use pane_host::HostEvent;
use protocol::PaneState;

/// The layout commands that keep persisted pane state in step with what the host reported.
pub(crate) fn commands_for(events: &[HostEvent]) -> Vec<Command> {
    events
        .iter()
        .filter_map(|event| match event {
            HostEvent::StateChanged { pane, state } => Some(Command::SetPaneState {
                pane: pane.clone(),
                state: state.clone(),
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
#[path = "event_commands_tests.rs"]
mod tests;
