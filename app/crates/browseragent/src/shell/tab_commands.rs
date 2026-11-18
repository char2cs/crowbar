use crate::shell::agent_command::AgentCommand;

/// The shell's side of the loop: it carries a command out on the thread that owns the tabs and
/// answers it through the shared [`Replies`](crate::Replies).
pub trait TabCommands: std::fmt::Debug + Send + Sync + 'static {
    /// Starts `command`. Never blocks; the answer comes later.
    fn run(
        &self,
        command: AgentCommand,
    );
}
