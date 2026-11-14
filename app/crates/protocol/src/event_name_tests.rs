use super::*;

#[test]
fn accepts_dotted_names_only() {
    for ok in ["review.updated", "a-b_c.d"] {
        assert!(EventName::new(ok).is_ok(), "{ok}");
    }
    for bad in ["", "a b", "a/b", "a\n", "x".repeat(129).as_str()] {
        assert!(EventName::new(bad).is_err(), "{bad:?}");
    }
}
