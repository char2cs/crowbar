const TAG_OUTPUT: u8 = 0;
const TAG_SNAPSHOT: u8 = 1;
const EXIT_TYPE: &str = "exit";
const UNKNOWN_EXIT_CODE: i32 = -1;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DaemonFrame {
    Output(Vec<u8>),
    Snapshot(Vec<u8>),
    Exit(i32),
}

impl DaemonFrame {
    pub fn from_binary(message: &[u8]) -> Option<Self> {
        let (tag, body) = message.split_first()?;
        match *tag {
            TAG_OUTPUT => Some(Self::Output(body.to_vec())),
            TAG_SNAPSHOT => Some(Self::Snapshot(body.to_vec())),
            _ => None,
        }
    }

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
    fn binary_frames_carry_a_tag() {
        assert_eq!(
            DaemonFrame::from_binary(b"\x00hi"),
            Some(DaemonFrame::Output(b"hi".to_vec()))
        );
        assert_eq!(
            DaemonFrame::from_binary(b"\x01hi"),
            Some(DaemonFrame::Snapshot(b"hi".to_vec()))
        );
        assert_eq!(DaemonFrame::from_binary(b"\x02hi"), None);
        assert_eq!(DaemonFrame::from_binary(b""), None);
    }

    #[test]
    fn text_frames_report_exit() {
        assert_eq!(
            DaemonFrame::from_text(r#"{"type":"exit","code":3}"#),
            Some(DaemonFrame::Exit(3))
        );
        assert_eq!(
            DaemonFrame::from_text(r#"{"type":"exit"}"#),
            Some(DaemonFrame::Exit(-1))
        );
        assert_eq!(DaemonFrame::from_text(r#"{"type":"ping"}"#), None);
        assert_eq!(DaemonFrame::from_text("nope"), None);
    }
}
