use crate::error::Result;
use crate::string_id::{check_text, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("event name", value, |c| {
        c.is_ascii_alphanumeric() || matches!(c, '.' | '-' | '_')
    })
}

string_id!(
    /// An event name declared in an extension manifest, for example `review.updated`.
    EventName,
    validate
);

#[cfg(test)]
#[path = "event_name_tests.rs"]
mod tests;
