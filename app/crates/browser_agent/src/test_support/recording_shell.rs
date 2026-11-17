use std::sync::Mutex;

use crate::agent_command::AgentCommand;
use crate::tab_commands::TabCommands;

#[derive(Debug, Default)]
pub(crate) struct RecordingShell {
    received: Mutex<Vec<AgentCommand>>,
}

impl RecordingShell {
    pub(crate) fn received(&self) -> Vec<AgentCommand> {
        self.received.lock().expect("the command list").clone()
    }
}

impl TabCommands for RecordingShell {
    fn run(
        &self,
        command: AgentCommand,
    ) {
        self.received
            .lock()
            .expect("the command list")
            .push(command);
    }
}
