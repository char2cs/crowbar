use browseragent::{AgentCommand, TabCommands};

use crate::request_queue::{Requests, send};
use crate::shell_request::ShellRequest;

#[derive(Debug)]
pub(crate) struct AgentRequests {
    requests: Requests,
}

impl AgentRequests {
    pub(crate) fn new(requests: Requests) -> Self {
        Self { requests }
    }
}

impl TabCommands for AgentRequests {
    fn run(
        &self,
        command: AgentCommand,
    ) {
        send(&self.requests, ShellRequest::Agent(command));
    }
}

#[cfg(test)]
mod tests {
    use browseragent::AgentOp;

    use super::*;

    #[test]
    fn a_command_from_the_daemon_becomes_a_request_of_the_shell() {
        let (requests, queue) = async_channel::unbounded();
        let command = AgentCommand {
            id: "1".into(),
            chat_id: "c1".into(),
            tab: "b1".into(),
            op: AgentOp::Close,
        };
        AgentRequests::new(requests).run(command.clone());
        assert_eq!(queue.try_recv(), Ok(ShellRequest::Agent(command)));
    }
}
