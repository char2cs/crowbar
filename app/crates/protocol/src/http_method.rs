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
#[path = "http_method_tests.rs"]
mod tests;
