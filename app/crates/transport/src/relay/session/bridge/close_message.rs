use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::tungstenite::protocol::CloseFrame;
use tokio_tungstenite::tungstenite::protocol::frame::coding::CloseCode;

pub(crate) fn close_message(
    code: CloseCode,
    reason: &'static str,
) -> Message {
    Message::Close(Some(CloseFrame {
        code,
        reason: reason.into(),
    }))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_close_carries_its_code_and_reason() {
        let Message::Close(Some(frame)) = close_message(CloseCode::Away, "gone") else {
            panic!("a close frame carries its code");
        };
        assert_eq!(frame.code, CloseCode::Away);
        assert_eq!(frame.reason.as_str(), "gone");
    }
}
