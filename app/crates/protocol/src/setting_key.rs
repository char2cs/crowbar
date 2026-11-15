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
mod tests {
    use super::*;

    #[test]
    fn accepts_dotted_names_only() {
        for ok in ["diff.contextLines", "a-b_c.d"] {
            assert!(SettingKey::new(ok).is_ok(), "{ok}");
        }
        for bad in ["", "a b", "a/b", "a\n", "x".repeat(129).as_str()] {
            assert!(SettingKey::new(bad).is_err(), "{bad:?}");
        }
    }
}
