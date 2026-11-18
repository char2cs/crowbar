use serde::Serialize;
use serde::de::DeserializeOwned;
use serde_json::Value;

use crate::control::Envelope;
use crate::control::PaneFrame;
use crate::control::SDK_VERSION;
use crate::control::ShellFrame;
use crate::error::{Error, Result};

/// Largest control frame accepted, in bytes. Bulk data uses the custom protocol instead.
pub const MAX_FRAME_BYTES: usize = 256 * 1024;

/// Encodes a frame as JSON text tagged with the SDK major version.
///
/// # Errors
/// [`Error::Json`] when the frame cannot be serialized and [`Error::FrameTooLarge`] when the
/// text exceeds [`MAX_FRAME_BYTES`].
pub fn encode<T: Serialize>(frame: &T) -> Result<String> {
    let text = serde_json::to_string(&Envelope {
        sdk: SDK_VERSION.major(),
        frame,
    })?;
    check_size(text.len())?;
    Ok(text)
}

/// Decodes a frame a pane sent.
///
/// # Errors
/// An [`Error`] when the text is too large, malformed, or speaks another SDK version.
pub fn decode_pane_frame(text: &str) -> Result<PaneFrame> {
    decode(text)
}

/// Decodes a frame the shell sent.
///
/// # Errors
/// An [`Error`] when the text is too large, malformed, or speaks another SDK version.
pub fn decode_shell_frame(text: &str) -> Result<ShellFrame> {
    decode(text)
}

fn check_size(size: usize) -> Result<()> {
    if size > MAX_FRAME_BYTES {
        return Err(Error::frame_too_large(size, MAX_FRAME_BYTES));
    }
    Ok(())
}

fn decode<T: DeserializeOwned>(text: &str) -> Result<T> {
    check_size(text.len())?;
    let mut value: Value = serde_json::from_str(text)?;
    let found = value
        .as_object_mut()
        .and_then(|object| object.remove("sdk"))
        .and_then(|sdk| sdk.as_u64());
    if found != Some(u64::from(SDK_VERSION.major())) {
        return Err(Error::unsupported_sdk(found, SDK_VERSION.major()));
    }
    Ok(serde_json::from_value(value)?)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_missing_or_foreign_sdk_version() {
        const CASES: [&str; 4] = [
            r#"{"kind":"pane.setTitle","title":"t"}"#,
            r#"{"sdk":2,"kind":"pane.setTitle","title":"t"}"#,
            r#"{"sdk":"1","kind":"pane.setTitle","title":"t"}"#,
            "[]",
        ];
        for text in CASES {
            assert!(
                matches!(decode_pane_frame(text), Err(Error::UnsupportedSdk { .. })),
                "{text}"
            );
        }
    }

    #[test]
    fn rejects_garbage_and_unknown_kinds() {
        const CASES: [&str; 2] = ["nope", r#"{"sdk":1,"kind":"pane.explode"}"#];
        for text in CASES {
            assert!(
                matches!(decode_pane_frame(text), Err(Error::Json(_))),
                "{text}"
            );
        }
    }

    #[test]
    fn rejects_oversized_text_before_parsing() {
        let text = " ".repeat(MAX_FRAME_BYTES + 1);
        assert!(
            matches!(decode_shell_frame(&text), Err(Error::FrameTooLarge { .. })),
            "oversized"
        );
    }

    #[test]
    fn encode_refuses_oversized_frames() {
        let frame = PaneFrame::Request {
            id: crate::RequestId::new(1),
            method: crate::MethodName::new("m").unwrap(),
            params: Value::String("x".repeat(MAX_FRAME_BYTES)),
        };
        assert!(
            matches!(encode(&frame), Err(Error::FrameTooLarge { .. })),
            "oversized"
        );
    }
}
