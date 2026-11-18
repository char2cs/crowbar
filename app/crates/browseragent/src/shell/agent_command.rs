use crate::shell::agent_op::AgentOp;

/// A request handed to the shell, which owns the tabs and answers through [`Replies`](crate::Replies).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AgentCommand {
    /// The request id the answer is filed under.
    pub id: String,
    /// The chat that made the call; the only chat whose tabs the call may touch.
    pub chat_id: String,
    /// The tab the call names; empty for `open`.
    pub tab: String,
    /// What to do.
    pub op: AgentOp,
}
