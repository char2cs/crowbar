use crate::error::{Error, Result};
use crate::string_id::{check_text, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("extension id", value, |c| {
        c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-'
    })?;
    if value.starts_with(|c: char| !c.is_ascii_lowercase()) {
        return Err(Error::malformed("extension id", "must start with a letter"));
    }
    Ok(())
}

string_id!(
    /// Identifies an extension: lower case words separated by `-`, starting with a letter.
    ExtensionId,
    validate
);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn starts_with_a_letter() {
        assert!(ExtensionId::new("git").is_ok(), "plain");
        assert!(ExtensionId::new("my-ext2").is_ok(), "digits and dashes");
        assert!(ExtensionId::new("2git").is_err(), "leading digit");
        assert!(ExtensionId::new("-git").is_err(), "leading dash");
        assert!(ExtensionId::new("Git").is_err(), "upper case");
    }
}
