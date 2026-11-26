use serde::{Deserialize, Serialize};
use serde_json::Value;

/// What the page, or the shell, answers an agent's request with.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub enum AgentAnswer {
    /// The request was carried out; the value is what the tool returns.
    Done(Value),
    /// The request failed; the text says why, for the agent to read.
    Failed(String),
}
