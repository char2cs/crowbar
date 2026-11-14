use crate::error::Result;
use crate::string_id::{check_text, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("command id", value, |c| {
        c.is_ascii_alphanumeric() || matches!(c, '.' | '-' | '_')
    })
}

string_id!(
    /// A shell command id, for example `ext.git.openReview`.
    CommandId,
    validate
);

#[cfg(test)]
#[path = "command_id_tests.rs"]
mod tests;
