use std::sync::Mutex;

use tokio::sync::Notify;

use crate::shell::AgentCommand;
use crate::shell::TabCommands;

#[derive(Debug, Default)]
pub(crate) struct RecordingShell {
    received: Mutex<Vec<AgentCommand>>,
    arrived: Notify,
}

impl RecordingShell {
    pub(crate) async fn first(&self) -> AgentCommand {
        self.arrived.notified().await;
        self.received.lock().expect("the command list").remove(0)
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
