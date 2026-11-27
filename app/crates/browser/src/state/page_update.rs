use serde::{Deserialize, Serialize};

use crate::address::PageUrl;
use crate::selection::PickReport;
use crate::state::agent_answer::AgentAnswer;

const MAX_TITLE_CHARS: usize = 200;
const MAX_FAILURE_CHARS: usize = 200;
const MAX_REPLY_BYTES: usize = 8 << 20;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "page", rename_all = "snake_case")]
#[non_exhaustive]
pub enum PageUpdate {
    Loading { url: String },
    Loaded { url: String },
    Title { title: String },
    Picked { report: PickReport },
    PickEnded,
    PickFailed { message: String },
    AgentReply { id: String, answer: AgentAnswer },
}

impl PageUpdate {
    #[must_use]
    pub fn title(title: &str) -> Self {
        Self::Title {
            title: title.chars().take(MAX_TITLE_CHARS).collect(),
        }
    }

    #[must_use]
    pub fn encode(&self) -> String {
        serde_json::to_string(self).unwrap_or_default()
    }

    #[must_use]
    pub fn decode(text: &str) -> Option<Self> {
        let update: Self = serde_json::from_str(text).ok()?;
        match &update {
            Self::Loading { url } | Self::Loaded { url } => {
                PageUrl::is_loadable(url).then_some(update)
            },
            Self::Title { .. }
            | Self::Picked { .. }
            | Self::PickEnded
            | Self::PickFailed { .. }
            | Self::AgentReply { .. } => Some(update),
        }
    }
    #[must_use]
    pub fn from_page_message(text: &str) -> Option<Self> {
        let message: serde_json::Value = serde_json::from_str(text).ok()?;
        if let Some(body) = message.get("crowbarPick") {
            return pick_update(body);
        }
        agent_update(message.get("crowbarAgent")?)
    }
}

fn failure(error: &str) -> PageUpdate {
    PageUpdate::PickFailed {
        message: error.chars().take(MAX_FAILURE_CHARS).collect(),
    }
}

fn agent_update(body: &serde_json::Value) -> Option<PageUpdate> {
    let id = body.get("id")?.as_str()?.to_owned();
    Some(PageUpdate::AgentReply {
        id,
        answer: answer_of(body)?,
    })
}

fn answer_of(body: &serde_json::Value) -> Option<AgentAnswer> {
    if body.to_string().len() > MAX_REPLY_BYTES {
        return Some(AgentAnswer::Failed(format!(
            "the page's answer is over {MAX_REPLY_BYTES} bytes"
        )));
    }
    if let Some(error) = body.get("error").and_then(serde_json::Value::as_str) {
        return Some(AgentAnswer::Failed(
            error.chars().take(MAX_FAILURE_CHARS).collect(),
        ));
    }
    body.get("result").cloned().map(AgentAnswer::Done)
}

fn pick_update(body: &serde_json::Value) -> Option<PageUpdate> {
    if let Some(error) = body.get("error").and_then(serde_json::Value::as_str) {
        return Some(failure(error));
    }
    let result = body.get("result")?;
    if result.is_null() {
        return Some(PageUpdate::PickEnded);
    }
    let report = serde_json::from_value(result.clone()).ok()?;
    Some(PageUpdate::Picked { report })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_update_survives_the_trip() {
        let cases = [
            PageUpdate::Loading {
                url: "https://a.test/".into(),
            },
            PageUpdate::Loaded {
                url: "about:blank".into(),
            },
            PageUpdate::title("Hello"),
        ];
        for update in cases {
            assert_eq!(PageUpdate::decode(&update.encode()), Some(update));
        }
    }

    #[test]
    fn foreign_text_and_unloadable_addresses_are_dropped() {
        let cases = [
            r#"{"sdk":1,"kind":"command.run","command":"x/y"}"#,
            r#"{"page":"loaded","url":"file:///etc/passwd"}"#,
            r#"{"page":"loading","url":"crowbar://core/x"}"#,
            "not json",
        ];
        for text in cases {
            assert_eq!(PageUpdate::decode(text), None, "{text}");
        }
    }

    fn kind_of(text: &str) -> &'static str {
        match PageUpdate::from_page_message(text) {
            Some(PageUpdate::AgentReply {
                answer: AgentAnswer::Done(_),
                ..
            }) => "answered",
            Some(PageUpdate::AgentReply {
                answer: AgentAnswer::Failed(_),
                ..
            }) => "refused",
            Some(PageUpdate::PickEnded) => "ended",
            Some(PageUpdate::PickFailed { .. }) => "failed",
            Some(PageUpdate::Picked { .. }) => "picked",
            _ => "none",
        }
    }

    #[test]
    fn only_the_messages_of_the_shells_own_scripts_are_read_from_a_page() {
        const CASES: &[(&str, &str)] = &[
            (r#"{"crowbarPick":{"result":null}}"#, "ended"),
            (r#"{"crowbarPick":{"error":"boom"}}"#, "failed"),
            (
                r#"{"crowbarPick":{"result":{"kind":"element","title":"t","url":"u","items":[],"more":0,"box":[1,2,3,4]}}}"#,
                "picked",
            ),
            (
                r#"{"crowbarAgent":{"id":"7","result":{"a":1}}}"#,
                "answered",
            ),
            (r#"{"crowbarAgent":{"id":"7","result":null}}"#, "answered"),
            (r#"{"crowbarAgent":{"id":"7","error":"boom"}}"#, "refused"),
            (r#"{"crowbarAgent":{"result":1}}"#, "none"),
            (r#"{"crowbarAgent":{"id":"7"}}"#, "none"),
            (r#"{"page":"loaded","url":"https://a.test/"}"#, "none"),
            (r#"{"crowbarPick":{"result":{"kind":1}}}"#, "none"),
            ("not json", "none"),
        ];
        for (text, expected) in CASES {
            assert_eq!(kind_of(text), *expected, "{text}");
        }
    }

    #[test]
    fn an_answer_too_large_to_carry_is_refused_by_id() {
        let body = serde_json::json!({
            "crowbarAgent": { "id": "9", "result": "x".repeat(MAX_REPLY_BYTES) }
        });
        let update = PageUpdate::from_page_message(&body.to_string());
        let refused = matches!(
            &update,
            Some(PageUpdate::AgentReply { id, answer: AgentAnswer::Failed(_) }) if id == "9"
        );
        assert!(refused, "a refusal for request 9, got {update:?}");
    }

    #[test]
    fn a_long_title_is_cut() {
        let PageUpdate::Title { title } = PageUpdate::title(&"x".repeat(1000)) else {
            panic!("a title update");
        };
        assert_eq!(title.chars().count(), 200);
    }
}
