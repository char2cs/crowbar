use std::sync::Mutex;

use tokio::sync::Notify;

use crate::agent_command::AgentCommand;
use crate::tab_commands::TabCommands;

#[derive(Debug, Default)]
pub(crate) struct RecordingShell {
    received: Mutex<Vec<AgentCommand>>,
    arrived: Notify,
}

impl RecordingShell {
    pub(crate) fn received(&self) -> Vec<AgentCommand> {
        self.received.lock().expect("the command list").clone()
    }

    pub(crate) async fn first(&self) -> AgentCommand {
        self.arrived.notified().await;
        self.received().remove(0)
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
        self.arrived.notify_one();
    }
}
