use serde::Deserialize;
use serde_json::Value;

#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
pub(crate) struct AgentRequest {
    pub(crate) id: String,
    pub(crate) op: String,
    #[serde(rename = "chatId")]
    pub(crate) chat_id: String,
    #[serde(default)]
    pub(crate) tab: String,
    #[serde(default)]
    pub(crate) args: Value,
}
