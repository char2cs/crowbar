use model::Command;
use pane_host::HostEvent;
use protocol::PaneState;

/// The layout commands the events of the pane host imply. Events that need a shell decision
/// (opening panes, running commands, extension requests) are logged by the caller instead.
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
