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
    pub fn equals(
        key: impl Into<Key>,
        value: impl Into<Value>,
    ) -> Self {
        Self::Equals(key.into(), value.into())
    }

    /// The platform offers `name`.
    pub fn capability(name: impl Into<String>) -> Self {
        Self::Capability(name.into())
    }

    pub(crate) fn holds(
        &self,
        scope: &Scope<'_>,
    ) -> bool {
        match self {
            Self::Equals(key, value) => scope.lookup(key) == Some(value),
            Self::Capability(name) => scope.capabilities.contains(name),
            Self::All(rules) => rules.iter().all(|rule| rule.holds(scope)),
            Self::Any(rules) => rules.iter().any(|rule| rule.holds(scope)),
        }
    }
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeSet;

    use super::*;
    use crate::values::Values;

    fn holds(condition: &Condition) -> bool {
        let values = Values::default().with("engine", "custom");
        let defaults = Values::default().with("zoom", 100.0);
        let capabilities = BTreeSet::from(["browser.clearData".to_owned()]);
        condition.holds(&Scope {
            values: &values,
            defaults: &defaults,
            capabilities: &capabilities,
        })
    }

    #[test]
    fn leaf_rules() {
        assert!(
            holds(&Condition::equals("engine", "custom")),
            "stored value"
        );
        assert!(holds(&Condition::equals("zoom", 100.0)), "default value");
        assert!(
            !holds(&Condition::equals("engine", "google")),
            "different value"
        );
        assert!(
            holds(&Condition::capability("browser.clearData")),
            "present capability"
        );
        assert!(
            !holds(&Condition::capability("hosts.add")),
            "absent capability"
        );
    }

    #[test]
    fn combinators() {
        let yes = Condition::equals("engine", "custom");
        let no = Condition::capability("nope");
        assert!(
            holds(&Condition::All(vec![yes.clone(), yes.clone()])),
            "all true"
        );
        assert!(
            !holds(&Condition::All(vec![yes.clone(), no.clone()])),
            "one false"
        );
        assert!(holds(&Condition::Any(vec![no.clone(), yes])), "one true");
        assert!(!holds(&Condition::Any(vec![no])), "none true");
        assert!(holds(&Condition::All(vec![])), "empty all");
        assert!(!holds(&Condition::Any(vec![])), "empty any");
    }
}
