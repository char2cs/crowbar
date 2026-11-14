use super::*;

#[test]
fn is_upper_case_letters_only() {
    for ok in ["GET", "POST", "PROPFIND"] {
        assert!(HttpMethod::new(ok).is_ok(), "{ok}");
    }
    for bad in ["", "get", "G ET", "GET\r\n", "G-T"] {
        assert!(HttpMethod::new(bad).is_err(), "{bad:?}");
    }
}
