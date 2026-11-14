use super::*;

#[test]
fn is_capped_but_may_be_empty() {
    assert!(DisplayText::new("").is_ok(), "empty");
    assert!(
        DisplayText::new("x".repeat(MAX_DISPLAY_TEXT_BYTES)).is_ok(),
        "at the cap"
    );
    assert!(
        DisplayText::new("x".repeat(MAX_DISPLAY_TEXT_BYTES + 1)).is_err(),
        "over the cap"
    );
}
