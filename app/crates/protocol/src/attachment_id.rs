use crate::error::Result;
use crate::string_id::{check_text, is_word_char, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("attachment id", value, is_word_char)
}

string_id!(
    /// Identifies one stored attachment, the argument of `attachment.open`.
    AttachmentId,
    validate
);

#[cfg(test)]
#[path = "attachment_id_tests.rs"]
mod tests;
