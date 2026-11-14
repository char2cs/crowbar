use super::*;

#[test]
fn rejects_empty_long_and_foreign_characters() {
    let any = |_: char| true;
    assert!(
        matches!(check_text("x", "", any), Err(Error::EmptyId { .. })),
        "empty"
    );
    let long = "a".repeat(MAX_ID_BYTES + 1);
    assert!(
        matches!(check_text("x", &long, any), Err(Error::IdTooLong { .. })),
        "long"
    );
    assert!(
        check_text("x", &"a".repeat(MAX_ID_BYTES), any).is_ok(),
        "at the cap"
    );
    assert!(
        matches!(
            check_text("x", "a/b", is_word_char),
            Err(Error::InvalidIdCharacter { found: '/', .. })
        ),
        "slash"
    );
}
