//! Websocket messages to and from the terminal types.

use terminal::{ClientMessage, DaemonFrame};
use tokio_tungstenite::tungstenite::Message;

pub(crate) fn frame_of(message: &Message) -> Option<DaemonFrame> {
    match message {
        Message::Binary(bytes) => DaemonFrame::from_binary(bytes),
        Message::Text(text) => DaemonFrame::from_text(text),
        _ => None,
    }
}

pub(crate) fn message_of(message: &ClientMessage) -> Message {
    Message::text(message.to_json())
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (&'static str, Message, Option<DaemonFrame>);

    fn cases() -> Vec<Case> {
        vec![
            (
                "output",
                Message::binary(vec![0, b'h', b'i']),
                Some(DaemonFrame::Output(b"hi".to_vec())),
            ),
            (
                "snapshot",
                Message::binary(vec![1, b'x']),
                Some(DaemonFrame::Snapshot(b"x".to_vec())),
            ),
            (
                "exit",
                Message::text(r#"{"type":"exit","code":3}"#),
                Some(DaemonFrame::Exit(3)),
            ),
            ("ping", Message::Ping(vec![1].into()), None),
        ]
    }

    #[test]
    fn websocket_messages_decode_to_frames() {
        for (name, message, expected) in cases() {
            assert_eq!(frame_of(&message), expected, "{name}");
        }
    }

    #[test]
    fn input_goes_out_as_json_text() {
        let sent = message_of(&ClientMessage::Input("ls\r".into()));
        let parsed: serde_json::Value = serde_json::from_str(sent.to_text().unwrap()).unwrap();
        assert_eq!(parsed["data"], "ls\r");
    }
}
