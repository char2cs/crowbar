use crate::shell::agent_command::AgentCommand;

pub trait TabCommands: std::fmt::Debug + Send + Sync + 'static {
    fn run(
        &self,
        command: AgentCommand,
    );
}
