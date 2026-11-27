use crate::shell::agent_op::AgentOp;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AgentCommand {
    pub id: String,
    pub chat_id: String,
    pub tab: String,
    pub op: AgentOp,
}
