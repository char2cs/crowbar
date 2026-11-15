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
mod tests {
    use super::*;

    #[test]
    fn accepts_word_characters_only() {
        assert!(AttachmentId::new("att:1.a-b_c").is_ok(), "word characters");
        assert!(AttachmentId::new("a b").is_err(), "space");
        assert!(AttachmentId::new("").is_err(), "empty");
    }
}
