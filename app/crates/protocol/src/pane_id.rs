use crate::error::Result;
use crate::string_id::{check_text, is_word_char, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("pane id", value, is_word_char)
}

string_id!(
    /// Identifies one pane instance for its whole life, including while it is parked.
    PaneId,
    validate
);

#[cfg(test)]
#[path = "pane_id_tests.rs"]
mod tests;
