use crate::error::{Error, Result};
use crate::string_id::{check_text, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("extension id", value, |c| {
        c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-'
    })?;
    if value.starts_with(|c: char| !c.is_ascii_lowercase()) {
        return Err(Error::MalformedId {
            kind: "extension id",
            reason: "must start with a letter",
        });
    }
    Ok(())
}

string_id!(
    /// Identifies an extension: lower case words separated by `-`, starting with a letter.
    ExtensionId,
    validate
);

#[cfg(test)]
#[path = "extension_id_tests.rs"]
mod tests;
