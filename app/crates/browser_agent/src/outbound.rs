use browser_nav::AgentAnswer;
use serde_json::json;

use crate::tab_info::TabInfo;

const TYPE: &str = "type";

pub(crate) fn tabs_frame(tabs: &[TabInfo]) -> String {
    json!({ TYPE: "tabs", "tabs": tabs }).to_string()
}

pub(crate) fn response_frame(
    id: &str,
    answer: &AgentAnswer,
) -> String {
    match answer {
        AgentAnswer::Done(result) => {
            json!({ TYPE: "response", "id": id, "ok": true, "result": result })
        },
        AgentAnswer::Failed(error) => {
            json!({ TYPE: "response", "id": id, "ok": false, "error": error })
        },
    }
    .to_string()
}

#[cfg(test)]
mod tests {
    use serde_json::{Value, json};

    use super::*;

    fn tab() -> TabInfo {
        TabInfo {
            id: "b1".into(),
            chat_id: "c1".into(),
            url: "https://a.test/".into(),
            title: "A".into(),
            loading: true,
        }
    }

    fn read(text: &str) -> Value {
        serde_json::from_str(text).unwrap()
    }

    #[test]
    fn frames_read_the_way_the_daemon_expects() {
        let cases = [
            (
                "the tab list names each tab",
                tabs_frame(&[tab()]),
                json!({"type":"tabs","tabs":[
                    {"id":"b1","chatId":"c1","url":"https://a.test/","title":"A","loading":true}
                ]}),
            ),
            (
                "an answer carries its result",
                response_frame("7", &AgentAnswer::Done(json!({"tab":"b1"}))),
                json!({"type":"response","id":"7","ok":true,"result":{"tab":"b1"}}),
            ),
            (
                "a null result is still a result",
                response_frame("7", &AgentAnswer::Done(Value::Null)),
                json!({"type":"response","id":"7","ok":true,"result":null}),
            ),
            (
                "a failure carries its error",
                response_frame("7", &AgentAnswer::Failed("no".into())),
                json!({"type":"response","id":"7","ok":false,"error":"no"}),
            ),
        ];
        for (name, frame, expected) in cases {
            assert_eq!(read(&frame), expected, "{name}");
        }
    }
}
