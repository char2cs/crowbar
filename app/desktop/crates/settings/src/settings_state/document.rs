//! The values on screen: stored values, in-flight writes and drafts being typed.

mod editing;
mod on_screen;
mod outcome;
mod reads;

use std::collections::BTreeMap;

use crate::settings_state::effect::Effect;
use crate::values::Key;
use crate::values::Value;
use crate::values::Values;

#[derive(Debug, Clone)]
pub(crate) struct Document {
    pub(crate) values: Values,
    pub(crate) defaults: Values,
    pub(crate) unsaved: BTreeMap<Key, Option<Value>>,
    pub(crate) drafts: BTreeMap<Key, String>,
}

impl Document {
    pub(crate) fn new(
        values: Values,
        defaults: Values,
    ) -> Self {
        Self {
            values,
            defaults,
            unsaved: BTreeMap::new(),
            drafts: BTreeMap::new(),
        }
    }

    pub(crate) fn apply(
        &mut self,
        key: &Key,
        value: Value,
    ) -> Effect {
        let previous = self.values.get(key).cloned();
        self.unsaved.entry(key.clone()).or_insert(previous);
        self.values.set(key.clone(), value.clone());
        self.drafts.remove(key);
        Effect::Write(key.clone(), value)
    }

    pub(crate) fn acknowledge(
        &mut self,
        key: &Key,
    ) {
        self.unsaved.remove(key);
    }

    pub(crate) fn fail(
        &mut self,
        key: &Key,
    ) {
        match self.unsaved.remove(key) {
            Some(Some(previous)) => self.values.set(key.clone(), previous),
            Some(None) => self.values.remove(key),
            None => {},
        }
        self.drafts.remove(key);
    }

    pub(crate) fn remote(
        &mut self,
        key: &Key,
        value: Value,
    ) {
        if !self.unsaved.contains_key(key) {
            self.values.set(key.clone(), value);
        }
    }

    pub(crate) fn edit(
        &mut self,
        key: &Key,
        text: &str,
    ) {
        self.drafts.insert(key.clone(), text.to_owned());
    }

    pub(crate) fn revert(
        &mut self,
        key: &Key,
    ) {
        self.drafts.remove(key);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn document() -> Document {
        Document::new(Values::default(), Values::default().with("tabSize", 2.0))
    }

    #[test]
    fn a_failed_write_restores_what_was_stored_before() {
        let mut document = document();
        let key = Key::from("tabSize");
        document.apply(&key, Value::Number(4.0));
        document.apply(&key, Value::Number(5.0));
        assert!(document.has_unsaved());
        document.fail(&key);
        assert!(
            document.values().get(&key).is_none(),
            "both writes are undone to the original state"
        );
        assert!(!document.has_unsaved());
        document.apply(&key, Value::Number(6.0));
        document.acknowledge(&key);
        document.apply(&key, Value::Number(7.0));
        document.fail(&key);
        assert_eq!(
            document.values().get(&key),
            Some(&Value::Number(6.0)),
            "back to the acknowledged value"
        );
    }

    #[test]
    fn remote_changes_do_not_override_a_write_in_flight() {
        let mut document = document();
        let key = Key::from("tabSize");
        document.apply(&key, Value::Number(4.0));
        document.remote(&key, Value::Number(8.0));
        assert_eq!(document.values().get(&key), Some(&Value::Number(4.0)));
        document.acknowledge(&key);
        document.remote(&key, Value::Number(8.0));
        assert_eq!(document.values().get(&key), Some(&Value::Number(8.0)));
    }
}
