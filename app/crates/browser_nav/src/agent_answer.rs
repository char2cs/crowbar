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

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn an_answer_survives_the_trip() {
        let cases = [
            AgentAnswer::Done(json!({"a": [1, null]})),
            AgentAnswer::Failed("no".into()),
        ];
        for answer in cases {
            let text = serde_json::to_string(&answer).unwrap();
            assert_eq!(serde_json::from_str::<AgentAnswer>(&text).unwrap(), answer);
        }
    }
}
