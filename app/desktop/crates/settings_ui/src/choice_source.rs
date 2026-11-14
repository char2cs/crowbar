//! Where a select gets its options.

use crate::choice::Choice;

/// Fixed options, or a named provider the backend answers at open (themes, fonts).
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum ChoiceSource {
    /// Options known to the schema.
    Fixed(Vec<Choice>),
    /// Options the backend lists under this name.
    Provided(String),
}

impl ChoiceSource {
    /// Fixed options from `(value, label)` pairs.
    pub fn fixed(pairs: &[(&str, &str)]) -> Self {
        Self::Fixed(
            pairs
                .iter()
                .map(|&(value, label)| Choice::new(value, label))
                .collect(),
        )
    }

    /// Options listed by the backend provider `name`.
    pub fn provided(name: impl Into<String>) -> Self {
        Self::Provided(name.into())
    }
}

#[cfg(test)]
#[path = "choice_source_tests.rs"]
mod tests;
