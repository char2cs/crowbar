use super::*;

#[test]
fn accepts_dotted_names_only() {
    for ok in ["excalidraw.drawing", "a-b_c.d"] {
        assert!(AttachmentType::new(ok).is_ok(), "{ok}");
    }
    for bad in ["", "a b", "a/b", "a\n", "x".repeat(129).as_str()] {
        assert!(AttachmentType::new(bad).is_err(), "{bad:?}");
    }
}
