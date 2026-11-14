use super::*;

#[test]
fn accepts_word_characters_only() {
    assert!(AttachmentId::new("att:1.a-b_c").is_ok(), "word characters");
    assert!(AttachmentId::new("a b").is_err(), "space");
    assert!(AttachmentId::new("").is_err(), "empty");
}
