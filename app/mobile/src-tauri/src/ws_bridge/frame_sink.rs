use tauri::ipc::{Channel, InvokeResponseBody};

use super::frame::Frame;

pub(crate) trait FrameSink: Send + Sync + 'static {
    fn send(
        &self,
        frame: Frame,
    );
}

fn body_of(frame: Frame) -> Option<InvokeResponseBody> {
    match frame {
        Frame::Text(text) => serde_json::to_string(&text)
            .ok()
            .map(InvokeResponseBody::Json),
        Frame::Binary(bytes) => Some(InvokeResponseBody::Raw(bytes)),
    }
}

impl FrameSink for Channel<InvokeResponseBody> {
    fn send(
        &self,
        frame: Frame,
    ) {
        let Some(body) = body_of(frame) else {
            return;
        };
        if let Err(error) = Channel::send(self, body) {
            log::debug!("ws bridge: webview channel is gone, frame dropped: {error}");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn text_travels_as_a_json_string_and_binary_as_raw_bytes() {
        let json = body_of(Frame::Text("a\"b".to_owned()));
        assert!(
            matches!(&json, Some(InvokeResponseBody::Json(text)) if text.as_str() == "\"a\\\"b\"")
        );
        let raw = body_of(Frame::Binary(vec![0, 255]));
        assert!(
            matches!(&raw, Some(InvokeResponseBody::Raw(bytes)) if bytes.as_slice() == [0, 255])
        );
    }
}
