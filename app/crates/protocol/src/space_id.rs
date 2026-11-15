use crate::error::Result;
use crate::string_id::{check_text, is_word_char, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("space id", value, is_word_char)
}

string_id!(
    /// Identifies a Space, the scope extension events can be limited to.
    SpaceId,
    validate
);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_word_characters_only() {
        assert!(SpaceId::new("s1").is_ok(), "plain");
        assert!(SpaceId::new("s/1").is_err(), "slash");
    }
}
