use crate::error::Result;
use crate::string_id::{check_text, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("attachment type", value, |c| {
        c.is_ascii_alphanumeric() || matches!(c, '.' | '-' | '_')
    })
}

string_id!(
    /// The id of an attachment type an extension contributes.
    AttachmentType,
    validate
);

#[cfg(test)]
#[path = "attachment_type_tests.rs"]
mod tests;
