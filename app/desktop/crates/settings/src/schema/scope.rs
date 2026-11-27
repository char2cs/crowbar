use std::collections::BTreeSet;

use crate::values::Key;
use crate::values::Value;
use crate::values::Values;

#[derive(Debug, Clone, Copy)]
pub(crate) struct Scope<'a> {
    pub(crate) values: &'a Values,
    pub(crate) defaults: &'a Values,
    pub(crate) capabilities: &'a BTreeSet<String>,
}

impl Scope<'_> {
    pub(crate) fn lookup(
        &self,
        key: &Key,
    ) -> Option<&Value> {
        self.values.get(key).or_else(|| self.defaults.get(key))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn stored_values_win_over_defaults() {
        let values = Values::default().with("a", Value::Bool(true));
        let defaults = Values::default()
            .with("a", Value::Bool(false))
            .with("b", 3.0);
        let capabilities = BTreeSet::new();
        let scope = Scope {
            values: &values,
            defaults: &defaults,
            capabilities: &capabilities,
        };
        assert_eq!(scope.lookup(&Key::from("a")), Some(&Value::Bool(true)));
        assert_eq!(scope.lookup(&Key::from("b")), Some(&Value::Number(3.0)));
        assert_eq!(scope.lookup(&Key::from("c")), None);
    }
}
