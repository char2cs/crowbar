use crate::error::Result;
use crate::string_id::{check_text, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("setting key", value, |c| {
        c.is_ascii_alphanumeric() || matches!(c, '.' | '-' | '_')
    })
}

string_id!(
    /// A key in a pane's settings namespace, for example `diff.contextLines`.
    SettingKey,
    validate
);

#[cfg(test)]
#[path = "setting_key_tests.rs"]
mod tests;
