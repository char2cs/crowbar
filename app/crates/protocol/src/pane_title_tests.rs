use super::*;

#[test]
fn is_bounded_and_free_of_control_characters() {
    assert!(PaneTitle::new("Chat \u{2014} main").is_ok(), "plain");
    assert!(PaneTitle::new("").is_ok(), "empty means default");
    assert!(
        PaneTitle::new("x".repeat(MAX_TITLE_BYTES + 1)).is_err(),
        "too long"
    );
    for bad in ["a\nb", "a\u{7}", "\u{1b}[31m"] {
        assert!(PaneTitle::new(bad).is_err(), "{bad:?}");
    }
}
