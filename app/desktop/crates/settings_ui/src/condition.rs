//! When a page, row or button applies.

use crate::key::Key;
use crate::scope::Scope;
use crate::value::Value;

/// A rule over setting values and platform capabilities.
#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum Condition {
    /// The setting currently holds this value (its default when unset).
    Equals(Key, Value),
    /// The platform offers the named capability, such as `browser.clearData`.
    Capability(String),
    /// Every rule holds.
    All(Vec<Condition>),
    /// At least one rule holds.
    Any(Vec<Condition>),
}

impl Condition {
    /// The setting `key` equals `value`.
    pub fn equals(key: impl Into<Key>, value: impl Into<Value>) -> Self {
        Self::Equals(key.into(), value.into())
    }

    /// The platform offers `name`.
    pub fn capability(name: impl Into<String>) -> Self {
        Self::Capability(name.into())
    }

    pub(crate) fn holds(&self, scope: &Scope<'_>) -> bool {
        match self {
            Self::Equals(key, value) => scope.lookup(key) == Some(value),
            Self::Capability(name) => scope.capabilities.contains(name),
            Self::All(rules) => rules.iter().all(|rule| rule.holds(scope)),
            Self::Any(rules) => rules.iter().any(|rule| rule.holds(scope)),
        }
    }
}

#[cfg(test)]
#[path = "condition_tests.rs"]
mod tests;
