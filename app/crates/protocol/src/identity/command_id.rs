use crate::error::Result;
use crate::string_id::{check_text, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("command id", value, |c| {
        c.is_ascii_alphanumeric() || matches!(c, '.' | '-' | '_')
    })
}

string_id!(CommandId, validate);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_dotted_names_only() {
        for ok in ["ext.git.openReview", "a-b_c.d"] {
            assert!(CommandId::new(ok).is_ok(), "{ok}");
        }
        for bad in ["", "a b", "a/b", "a\n", "x".repeat(129).as_str()] {
            assert!(CommandId::new(bad).is_err(), "{bad:?}");
        }
    }
}
