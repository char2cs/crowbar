//! Frames the daemon sends over a terminal websocket.

const TAG_OUTPUT: u8 = 0;
const TAG_SNAPSHOT: u8 = 1;
const EXIT_TYPE: &str = "exit";
const UNKNOWN_EXIT_CODE: i32 = -1;

/// One message of the PTY stream (api/internal/core/terminal/transport.go).
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DaemonFrame {
    /// Bytes the program wrote; append them.
    Output(Vec<u8>),
    /// A ground-state redraw of the whole screen; replaces what is shown.
    Snapshot(Vec<u8>),
    /// The session's process ended.
    Exit(i32),
}

impl DaemonFrame {
    /// Decodes a binary message: one tag byte then raw bytes. Unknown tags and empty messages give `None`.
    pub fn from_binary(message: &[u8]) -> Option<Self> {
        let (tag, body) = message.split_first()?;
        match *tag {
            TAG_OUTPUT => Some(Self::Output(body.to_vec())),
            TAG_SNAPSHOT => Some(Self::Snapshot(body.to_vec())),
            _ => None,
        }
    }

    /// Decodes a text message, `{"type":"exit","code":N}`; anything else gives `None`.
    pub fn from_text(message: &str) -> Option<Self> {
        let payload: serde_json::Value = serde_json::from_str(message).ok()?;
        let kind = payload.get("type")?.as_str()?;
        (kind == EXIT_TYPE).then(|| Self::Exit(exit_code(&payload)))
    }
}

fn exit_code(value: &serde_json::Value) -> i32 {
    value
        .get("code")
        .and_then(serde_json::Value::as_i64)
        .and_then(|code| i32::try_from(code).ok())
        .unwrap_or(UNKNOWN_EXIT_CODE)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn frames_decode_by_tag_and_type() {
        let cases = [
            (
                DaemonFrame::from_binary(b"\x00hi"),
                Some(DaemonFrame::Output(b"hi".to_vec())),
            ),
            (
                DaemonFrame::from_binary(b"\x01hi"),
                Some(DaemonFrame::Snapshot(b"hi".to_vec())),
            ),
            (DaemonFrame::from_binary(b"\x02hi"), None),
            (DaemonFrame::from_binary(b""), None),
            (
                DaemonFrame::from_text(r#"{"type":"exit","code":3}"#),
                Some(DaemonFrame::Exit(3)),
            ),
            (
                DaemonFrame::from_text(r#"{"type":"exit"}"#),
                Some(DaemonFrame::Exit(-1)),
            ),
            (DaemonFrame::from_text(r#"{"type":"ping"}"#), None),
            (DaemonFrame::from_text("nope"), None),
        ];
        for (index, (decoded, expected)) in cases.into_iter().enumerate() {
            assert_eq!(decoded, expected, "case {index}");
        }
    }
}
