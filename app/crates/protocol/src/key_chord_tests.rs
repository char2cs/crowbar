use super::*;

#[test]
fn is_printable_ascii_without_spaces() {
    for ok in ["mod+k", "mod+shift+,", "f5"] {
        assert!(KeyChord::new(ok).is_ok(), "{ok}");
    }
    for bad in ["", "mod k", "mod+\u{e9}", "a\tb"] {
        assert!(KeyChord::new(bad).is_err(), "{bad:?}");
    }
    assert!(KeyChord::new("k".repeat(65)).is_err(), "too long");
}
