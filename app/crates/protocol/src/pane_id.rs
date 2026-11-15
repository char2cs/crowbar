use crate::error::Result;
use crate::string_id::{check_text, is_word_char, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("pane id", value, is_word_char)
}

string_id!(
    /// Identifies one pane instance for its whole life, including while it is parked.
    PaneId,
    validate
);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_word_characters_only() {
        assert!(PaneId::new("pane:1.a-b_c").is_ok(), "word characters");
        assert!(PaneId::new("a b").is_err(), "space");
    }
}
