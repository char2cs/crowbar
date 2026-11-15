use crate::error::Result;
use crate::string_id::{check_text, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("http method", value, |c| c.is_ascii_uppercase())
}

string_id!(
    /// An HTTP method in upper case letters, for example `GET`, at most 128 bytes.
    HttpMethod,
    validate
);

#[cfg(test)]
mod tests {
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
}
