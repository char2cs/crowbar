//! The values on screen: stored values, in-flight writes and drafts being typed.

use std::collections::BTreeMap;

use crate::effect::Effect;
use crate::key::Key;
use crate::row::Row;
use crate::value::Value;
use crate::values::Values;

#[derive(Debug, Clone)]
pub(crate) struct Document {
    values: Values,
    defaults: Values,
    unsaved: BTreeMap<Key, Option<Value>>,
    drafts: BTreeMap<Key, String>,
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

    pub(crate) fn value(
        &self,
        key: &Key,
    ) -> Option<&Value> {
        self.values.get(key).or_else(|| self.defaults.get(key))
    }

    pub(crate) fn values(&self) -> &Values {
        &self.values
    }

    pub(crate) fn defaults(&self) -> &Values {
        &self.defaults
    }

    pub(crate) fn is_default(
        &self,
        row: &Row,
    ) -> bool {
        row.default
            .as_ref()
            .is_none_or(|default| self.value(&row.key) == Some(default))
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

    pub(crate) fn text(
        &self,
        row: &Row,
    ) -> String {
        match self.drafts.get(&row.key) {
            Some(draft) => draft.clone(),
            None => row.control.text_of(self.value(&row.key)),
        }
    }

    pub(crate) fn draft(
        &self,
        key: &Key,
    ) -> Option<&str> {
        self.drafts.get(key).map(String::as_str)
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

    pub(crate) fn is_dirty(
        &self,
        row: &Row,
    ) -> bool {
        self.drafts
            .get(&row.key)
            .is_some_and(|draft| *draft != row.control.text_of(self.value(&row.key)))
    }

    pub(crate) fn has_unsaved(&self) -> bool {
        !self.unsaved.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::control::Control;
    use crate::control_width::ControlWidth;
    use crate::number_range::NumberRange;
    use crate::test_support::{Case, run_cases};

    fn size_row() -> Row {
        Row::new(
            "tabSize",
            "Tab Size",
            Control::number(NumberRange::new(1.0, 8.0, 1.0), ControlWidth::NumberCompact),
        )
        .default_value(2.0)
    }

    fn document() -> Document {
        Document::new(Values::default(), Values::default().with("tabSize", 2.0))
    }

    fn defaults_show_until_a_value_is_stored() {
        let mut document = document();
        let row = size_row();
        assert_eq!(document.value(&row.key), Some(&Value::Number(2.0)));
        assert!(document.values().get(&row.key).is_none());
        assert!(document.is_default(&row));
        document.apply(&row.key, Value::Number(4.0));
        assert!(!document.is_default(&row));
        assert_eq!(document.text(&row), "4");
    }

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

    fn drafts_track_dirtiness_until_reverted_or_applied() {
        let mut document = document();
        let row = size_row();
        document.edit(&row.key, "5");
        assert!(document.is_dirty(&row));
        assert_eq!(document.text(&row), "5");
        assert_eq!(document.draft(&row.key), Some("5"));
        document.edit(&row.key, "2");
        assert!(
            !document.is_dirty(&row),
            "typing the stored value back is not dirty"
        );
        document.revert(&row.key);
        assert_eq!(document.draft(&row.key), None);
        document.edit(&row.key, "7");
        document.apply(&row.key, Value::Number(7.0));
        assert_eq!(document.draft(&row.key), None, "applying clears the draft");
        assert_eq!(document.values().get(&row.key), Some(&Value::Number(7.0)));
        assert_eq!(document.defaults().get(&row.key), Some(&Value::Number(2.0)));
    }

    const CASES: &[Case] = &[
        (
            "defaults show until a value is stored",
            defaults_show_until_a_value_is_stored,
        ),
        (
            "a failed write restores what was stored before",
            a_failed_write_restores_what_was_stored_before,
        ),
        (
            "remote changes do not override a write in flight",
            remote_changes_do_not_override_a_write_in_flight,
        ),
        (
            "drafts track dirtiness until reverted or applied",
            drafts_track_dirtiness_until_reverted_or_applied,
        ),
    ];

    #[test]
    fn every_scenario_holds() {
        run_cases(CASES);
    }
}
