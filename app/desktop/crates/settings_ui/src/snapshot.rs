//! What the modal reads from the backend when it opens.

use std::collections::{BTreeMap, BTreeSet};

use crate::choice::Choice;
use crate::values::Values;

/// Values, platform capabilities and dynamic options, all known before the first paint.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct Snapshot {
    pub(crate) values: Values,
    pub(crate) capabilities: BTreeSet<String>,
    pub(crate) choices: BTreeMap<String, Vec<Choice>>,
}

impl Snapshot {
    /// A snapshot holding `values`.
    pub fn new(values: Values) -> Self {
        Self {
            values,
            ..Self::default()
        }
    }

    /// The platform offers `name`.
    #[must_use]
    pub fn with_capability(mut self, name: &str) -> Self {
        self.capabilities.insert(name.to_owned());
        self
    }

    /// The backend lists `choices` for the provider `provider`.
    #[must_use]
    pub fn with_choices(mut self, provider: &str, choices: Vec<Choice>) -> Self {
        self.choices.insert(provider.to_owned(), choices);
        self
    }
}

#[cfg(test)]
#[path = "snapshot_tests.rs"]
mod tests;
