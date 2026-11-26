//! When a page, row or button applies.

use crate::schema::scope::Scope;
use crate::values::Key;
use crate::values::Value;

#[derive(Debug, Clone, PartialEq)]
pub(crate) enum Condition {
    Equals(Key, Value),
    Capability(String),
}

impl Condition {
    pub(crate) fn equals(
        key: impl Into<Key>,
        value: impl Into<Value>,
    ) -> Self {
        Self::Equals(key.into(), value.into())
    }

    pub(crate) fn capability(name: impl Into<String>) -> Self {
        Self::Capability(name.into())
    }

    pub(crate) fn holds(
        &self,
        scope: &Scope<'_>,
    ) -> bool {
        match self {
            Self::Equals(key, value) => scope.lookup(key) == Some(value),
            Self::Capability(name) => scope.capabilities.contains(name),
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
}
