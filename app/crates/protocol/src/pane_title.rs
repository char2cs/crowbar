use crate::error::{Error, Result};
use crate::string_id::string_id;

/// Longest tab title, in bytes.
pub const MAX_TITLE_BYTES: usize = 256;

fn validate(value: &str) -> Result<()> {
    if value.len() > MAX_TITLE_BYTES {
        return Err(Error::IdTooLong {
            kind: "pane title",
            max: MAX_TITLE_BYTES,
        });
    }
    match value.chars().find(|c| c.is_control()) {
        Some(found) => Err(Error::InvalidIdCharacter {
            kind: "pane title",
            found,
        }),
        None => Ok(()),
    }
}

string_id!(
    /// The text of a pane's tab: at most [`MAX_TITLE_BYTES`] bytes, no control characters.
    /// Empty means the shell shows the pane type's default title.
    PaneTitle,
    validate
);

#[cfg(test)]
#[path = "pane_title_tests.rs"]
mod tests;
