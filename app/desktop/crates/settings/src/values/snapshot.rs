//! What the modal reads from the backend when it opens.

use std::collections::{BTreeMap, BTreeSet};

use crate::values::Values;
use crate::values::choice::Choice;

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
    pub fn with_capability(
        mut self,
        name: &str,
    ) -> Self {
        self.capabilities.insert(name.to_owned());
        self
    }

    /// The backend lists `choices` for the provider `provider`.
    #[must_use]
    pub fn with_choices(
        mut self,
        provider: &str,
        choices: Vec<Choice>,
    ) -> Self {
        self.choices.insert(provider.to_owned(), choices);
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::values::value::Value;

    #[test]
    fn builders_collect_capabilities_and_choices() {
        let snapshot = Snapshot::new(Values::default().with("a", Value::Bool(true)))
            .with_capability("browser.clearData")
            .with_choices("themes", vec![Choice::new("crowbar", "Crowbar")]);
        assert!(snapshot.capabilities.contains("browser.clearData"));
        assert_eq!(snapshot.choices.get("themes").map(Vec::len), Some(1));
        assert!(snapshot.values.get(&"a".into()).is_some());
    }
}
