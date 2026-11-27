use serde::Deserialize;
use serde_json::Value;

use crate::problem::Problem;

#[derive(Debug, Clone, PartialEq, Deserialize)]
#[serde(tag = "t", rename_all = "lowercase")]
pub enum Frame {
    Ready {
        epoch: String,
    },
    Entity {
        scope: String,
        kind: String,
        id: String,
        #[serde(default)]
        v: u64,
        #[serde(default)]
        data: Option<Value>,
        #[serde(default)]
        gone: bool,
    },
    Synced {
        scope: String,
        #[serde(default)]
        unread: Vec<String>,
    },
    Closed {
        scope: String,
        reason: String,
        #[serde(default)]
        problem: Option<Problem>,
    },
    Error {
        problem: Problem,
    },
    Row {
        scope: String,
    },
    Delta {
        scope: String,
    },
    Signal {
        scope: String,
        #[serde(default)]
        kind: String,
        #[serde(default)]
        data: Option<Value>,
    },
}

impl Frame {
    pub fn decode(text: &str) -> Result<Self, serde_json::Error> {
        serde_json::from_str(text)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_frame_type_the_projects_scope_uses_decodes() {
        let cases = [
            r#"{"t":"ready","epoch":"e1"}"#,
            r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":3,"data":{"id":"c"}}"#,
            r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":4,"gone":true}"#,
            r#"{"t":"synced","scope":"projects","unread":["r"]}"#,
            r#"{"t":"closed","scope":"projects","reason":"resync"}"#,
            r#"{"t":"error","problem":{"code":"x","message":"y"}}"#,
            r#"{"t":"row","scope":"chat:c","row":{},"from":1}"#,
            r#"{"t":"delta","scope":"chat:c","delta":{}}"#,
            r#"{"t":"signal","scope":"git:w","kind":"git_status"}"#,
        ];
        for text in cases {
            assert!(Frame::decode(text).is_ok(), "{text}");
        }
    }

    #[test]
    fn an_unknown_type_is_an_error() {
        assert!(Frame::decode(r#"{"t":"gossip"}"#).is_err());
        assert!(Frame::decode("nope").is_err());
    }

    #[test]
    fn a_signal_carries_its_kind_and_data() {
        let frame = Frame::decode(
            r#"{"t":"signal","scope":"files:w","kind":"file_change","data":{"type":"deleted","path":"a"}}"#,
        );
        let Ok(Frame::Signal { kind, data, .. }) = frame else {
            panic!("not a signal: {frame:?}");
        };
        assert_eq!(kind, "file_change");
        assert_eq!(data, Some(serde_json::json!({"type":"deleted","path":"a"})));
    }

    #[test]
    fn a_removal_carries_no_data() {
        let frame = Frame::decode(
            r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":4,"gone":true}"#,
        );
        assert!(matches!(
            frame,
            Ok(Frame::Entity {
                gone: true,
                data: None,
                ..
            })
        ));
    }
}
