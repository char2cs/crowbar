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
mod tests {
    use super::*;

    #[test]
    fn is_bounded_and_free_of_control_characters() {
        assert!(PaneTitle::new("Chat \u{2014} main").is_ok(), "plain");
        assert!(PaneTitle::new("").is_ok(), "empty means default");
        assert!(
            PaneTitle::new("x".repeat(MAX_TITLE_BYTES + 1)).is_err(),
            "too long"
        );
        for bad in ["a\nb", "a\u{7}", "\u{1b}[31m"] {
            assert!(PaneTitle::new(bad).is_err(), "{bad:?}");
        }
    }
}
