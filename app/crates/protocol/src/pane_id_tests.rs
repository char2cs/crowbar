use super::*;

#[test]
fn accepts_word_characters_only() {
    assert!(PaneId::new("pane:1.a-b_c").is_ok(), "word characters");
    assert!(PaneId::new("a b").is_err(), "space");
}
