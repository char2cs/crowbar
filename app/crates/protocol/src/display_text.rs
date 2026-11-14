use crate::error::{Error, Result};
use crate::string_id::string_id;

/// Longest human readable text a frame carries, in bytes.
pub const MAX_DISPLAY_TEXT_BYTES: usize = 1024;

fn validate(value: &str) -> Result<()> {
    if value.len() > MAX_DISPLAY_TEXT_BYTES {
        return Err(Error::IdTooLong {
            kind: "display text",
            max: MAX_DISPLAY_TEXT_BYTES,
        });
    }
    Ok(())
}

string_id!(
    /// Human readable text of at most [`MAX_DISPLAY_TEXT_BYTES`] bytes, such as an error message
    /// or the reason an extension is not running. It may be empty.
    DisplayText,
    validate
);

#[cfg(test)]
#[path = "display_text_tests.rs"]
mod tests;
