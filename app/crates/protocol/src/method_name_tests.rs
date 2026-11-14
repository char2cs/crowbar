use super::*;

#[test]
fn accepts_dotted_names_only() {
    for ok in ["review.diff", "a-b_c.d"] {
        assert!(MethodName::new(ok).is_ok(), "{ok}");
    }
    for bad in ["", "a b", "a/b", "a\n", "x".repeat(129).as_str()] {
        assert!(MethodName::new(bad).is_err(), "{bad:?}");
    }
}
