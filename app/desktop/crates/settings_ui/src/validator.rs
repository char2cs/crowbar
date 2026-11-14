//! Checks applied to typed text before it counts as valid.

use crate::issue::Issue;

/// A named check for a text field.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Validator {
    /// An http(s) address with a `%s` where the query goes.
    SearchTemplate,
}

impl Validator {
    /// Whether `text` is acceptable; empty text always is, because it means "unset".
    ///
    /// # Errors
    /// Returns the [`Issue`] found in a non-empty `text`.
    pub fn check(self, text: &str) -> Result<(), Issue> {
        match self {
            Self::SearchTemplate => check_template(text),
        }
    }
}

fn check_template(text: &str) -> Result<(), Issue> {
    if text.is_empty() {
        return Ok(());
    }
    let rest = text
        .strip_prefix("https://")
        .or_else(|| text.strip_prefix("http://"));
    let usable = rest.is_some_and(|rest| !rest.is_empty())
        && !text.chars().any(char::is_whitespace)
        && text.contains("%s");
    if usable {
        Ok(())
    } else {
        Err(Issue::BadSearchTemplate)
    }
}

#[cfg(test)]
#[path = "validator_tests.rs"]
mod tests;
