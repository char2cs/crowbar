//! A message of the daemon's `/v0/stream`.

use serde::Deserialize;
use serde_json::Value;

use crate::problem::Problem;

/// A message of the daemon's `/v0/stream`, told apart by its `t` field.
#[derive(Debug, Clone, PartialEq, Deserialize)]
#[serde(tag = "t", rename_all = "lowercase")]
pub enum Frame {
    /// The first frame of a connection; a new epoch voids every held version.
    Ready {
        /// The daemon's epoch.
        epoch: String,
    },
    /// A versioned entity of a scope, or its removal.
    Entity {
        /// The scope that carries it.
        scope: String,
        /// The entity kind, such as `chat`.
        kind: String,
        /// The entity id.
        id: String,
        /// Its version.
        #[serde(default)]
        v: u64,
        /// Its payload, absent for a removal.
        #[serde(default)]
        data: Option<Value>,
        /// The entity is gone.
        #[serde(default)]
        gone: bool,
    },
    /// The scope's snapshot is complete, save for the parts `unread` names.
    Synced {
        /// The scope.
        scope: String,
        /// Repos or projects whose rows the snapshot could not read.
        #[serde(default)]
        unread: Vec<String>,
    },
    /// The daemon ended the hold on a scope.
    Closed {
        /// The scope.
        scope: String,
        /// `resync`, `deleted` or `refused`.
        reason: String,
        /// Why, for a refusal.
        #[serde(default)]
        problem: Option<Problem>,
    },
    /// A refusal that names no scope.
    Error {
        /// Why.
        problem: Problem,
    },
    /// A chat ledger row.
    Row {
        /// The scope.
        scope: String,
    },
    /// Streamed text of a chat.
    Delta {
        /// The scope.
        scope: String,
    },
    /// An unversioned fact about a scope, such as a file change.
    Signal {
        /// The scope.
        scope: String,
        /// What the fact is, such as `file_change`.
        #[serde(default)]
        kind: String,
        /// Its payload, whose shape the kind names.
        #[serde(default)]
        data: Option<Value>,
    },
}

impl Frame {
    /// Parses one text message of the stream as a frame.
    ///
    /// # Errors
    ///
    /// A `serde_json::Error` when the text is not JSON or not a frame type this enum knows.
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
