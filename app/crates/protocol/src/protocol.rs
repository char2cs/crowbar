use serde::Serialize;
use serde::de::DeserializeOwned;
use serde_json::Value;

use crate::envelope::Envelope;
use crate::error::{Error, Result};
use crate::pane_frame::PaneFrame;
use crate::shell_frame::ShellFrame;
use crate::version::SDK_MAJOR;

/// Largest control frame accepted, in bytes. Bulk data uses the custom protocol instead.
pub const MAX_FRAME_BYTES: usize = 256 * 1024;

/// Encodes a frame as JSON text tagged with the SDK major version.
///
/// # Errors
/// [`Error::Json`] when the frame cannot be serialized and [`Error::FrameTooLarge`] when the
/// text exceeds [`MAX_FRAME_BYTES`].
pub fn encode<T: Serialize>(frame: &T) -> Result<String> {
    let text = serde_json::to_string(&Envelope {
        sdk: SDK_MAJOR,
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
        return Err(Error::FrameTooLarge {
            size,
            limit: MAX_FRAME_BYTES,
        });
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
    if found != Some(u64::from(SDK_MAJOR)) {
        return Err(Error::UnsupportedSdk {
            found,
            expected: SDK_MAJOR,
        });
    }
    Ok(serde_json::from_value(value)?)
}

#[cfg(test)]
#[path = "protocol_tests.rs"]
mod tests;
