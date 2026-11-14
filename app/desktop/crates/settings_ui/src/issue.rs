//! Why a typed value is not acceptable.

/// A problem found in text a user typed into a field.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Issue {
    /// The text is not a number.
    NotANumber,
    /// The text is not an http(s) address with a `%s` placeholder.
    BadSearchTemplate,
}
