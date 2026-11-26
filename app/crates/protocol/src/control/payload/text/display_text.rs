use crate::error::{Error, Result};
use crate::string_id::string_id;

const MAX_DISPLAY_TEXT_BYTES: usize = 1024;

fn validate(value: &str) -> Result<()> {
    if value.len() > MAX_DISPLAY_TEXT_BYTES {
        return Err(Error::too_long("display text", MAX_DISPLAY_TEXT_BYTES));
    }
    Ok(())
}

string_id!(
    /// Human readable text of at most 1024 bytes, such as an error message
    /// or the reason an extension is not running. It may be empty.
    DisplayText,
    validate
);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn is_capped_but_may_be_empty() {
        assert!(DisplayText::new("").is_ok(), "empty");
        assert!(
            DisplayText::new("x".repeat(MAX_DISPLAY_TEXT_BYTES)).is_ok(),
            "at the cap"
        );
        assert!(
            DisplayText::new("x".repeat(MAX_DISPLAY_TEXT_BYTES + 1)).is_err(),
            "over the cap"
        );
    }
}
