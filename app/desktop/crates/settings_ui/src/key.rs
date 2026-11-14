//! The name of a setting.

use std::fmt;

/// A setting key such as `uiFontSize` or `ext.git.autoFetch`; extension keys start with `ext.<id>.`.
#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct Key(String);

impl Key {
    /// A key named `name`.
    pub fn new(name: impl Into<String>) -> Self {
        Self(name.into())
    }

    /// The key as text.
    pub fn as_str(&self) -> &str {
        &self.0
    }

    pub(crate) fn is_in(&self, namespace: &str) -> bool {
        self.0
            .strip_prefix(namespace)
            .is_some_and(|rest| rest.starts_with('.'))
    }
}

impl From<&str> for Key {
    fn from(name: &str) -> Self {
        Self::new(name)
    }
}

impl fmt::Display for Key {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.0)
    }
}

#[cfg(test)]
#[path = "key_tests.rs"]
mod tests;
