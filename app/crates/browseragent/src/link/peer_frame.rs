use serde_json::Value;

use crate::link::agent_request::AgentRequest;

const HELLO: &str = "hello";
const REQUEST: &str = "request";

pub(crate) enum PeerFrame {
    Hello,
    Request(AgentRequest),
}

impl PeerFrame {
    pub(crate) fn decode(text: &str) -> Option<Self> {
        let head: Value = serde_json::from_str(text).ok()?;
        match head.get("type")?.as_str()? {
            HELLO => Some(Self::Hello),
            REQUEST => request_of(text),
            _ => None,
        }
    }
}

fn request_of(text: &str) -> Option<PeerFrame> {
    let request: AgentRequest = serde_json::from_str(text).ok()?;
    let complete = !request.id.is_empty() && !request.op.is_empty() && !request.chat_id.is_empty();
    complete.then_some(PeerFrame::Request(request))
}

#[cfg(test)]
mod tests {
    use super::*;

    const REQUEST_TEXT: &str =
        r#"{"type":"request","id":"4","op":"click","chatId":"c1","tab":"b1","args":{"ref":"e1"}}"#;

    const CASES: &[(&str, bool)] = &[
        (r#"{"type":"hello"}"#, true),
        (REQUEST_TEXT, true),
        (
            r#"{"type":"request","id":"","op":"click","chatId":"c1"}"#,
            false,
        ),
        (
            r#"{"type":"request","id":"4","op":"click","chatId":""}"#,
            false,
        ),
        (r#"{"type":"request","id":"4","chatId":"c1"}"#, false),
        (r#"{"type":"tabs"}"#, false),
        ("[1]", false),
        ("not json", false),
    ];

    fn read_request(text: &str) -> Option<AgentRequest> {
        match PeerFrame::decode(text)? {
            PeerFrame::Request(request) => Some(request),
            PeerFrame::Hello => None,
        }
    }

    #[test]
    fn the_frames_the_daemon_sends_are_read_and_the_rest_dropped() {
        for (text, read) in CASES {
            assert_eq!(PeerFrame::decode(text).is_some(), *read, "{text}");
        }
    }

    #[test]
    fn a_request_keeps_what_it_names_and_may_leave_the_tab_and_arguments_out() {
        let full = read_request(REQUEST_TEXT).unwrap();
        assert_eq!(
            (full.id.as_str(), full.op.as_str(), full.chat_id.as_str()),
            ("4", "click", "c1")
        );
        assert_eq!(
            (full.tab.as_str(), full.args["ref"].as_str()),
            ("b1", Some("e1"))
        );
        let bare =
            read_request(r#"{"type":"request","id":"1","op":"open","chatId":"c1","args":null}"#)
                .unwrap();
        assert_eq!((bare.tab.as_str(), bare.args), ("", Value::Null));
    }
}
