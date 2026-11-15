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
mod tests {
    use super::*;

    #[test]
    fn accepts_dotted_names_only() {
        for ok in ["excalidraw.drawing", "a-b_c.d"] {
            assert!(AttachmentType::new(ok).is_ok(), "{ok}");
        }
        for bad in ["", "a b", "a/b", "a\n", "x".repeat(129).as_str()] {
            assert!(AttachmentType::new(bad).is_err(), "{bad:?}");
        }
    }
}
