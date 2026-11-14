use super::*;

#[test]
fn accepts_word_characters_only() {
    assert!(SpaceId::new("s1").is_ok(), "plain");
    assert!(SpaceId::new("s/1").is_err(), "slash");
}
