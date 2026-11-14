use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};

/// The parameters a pane instance is identified by (`spaceId`, `threadId`, ...).
///
/// Values are ids only: panes fetch their data from the daemon. The map is ordered so equal
/// parameters always serialize and compare the same way.
#[derive(Debug, Clone, Default, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(transparent)]
pub struct Params(BTreeMap<String, String>);

impl Params {
    /// Adds or replaces one entry.
    #[must_use]
    pub fn with(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.0.insert(key.into(), value.into());
        self
    }

    /// Looks up one entry.
    #[must_use]
    pub fn get(&self, key: &str) -> Option<&str> {
        self.0.get(key).map(String::as_str)
    }

    /// Whether there are no entries.
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }

    /// Iterates entries in key order.
    pub fn iter(&self) -> impl Iterator<Item = (&str, &str)> {
        self.0.iter().map(|(k, v)| (k.as_str(), v.as_str()))
    }
}

#[cfg(test)]
#[path = "params_tests.rs"]
mod tests;
