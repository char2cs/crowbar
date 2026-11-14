use crate::error::Result;
use crate::string_id::{check_text, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("method name", value, |c| {
        c.is_ascii_alphanumeric() || matches!(c, '.' | '-' | '_')
    })
}

string_id!(
    /// A method an extension backend serves, as declared in its manifest, for example `review.diff`.
    MethodName,
    validate
);

#[cfg(test)]
#[path = "method_name_tests.rs"]
mod tests;
