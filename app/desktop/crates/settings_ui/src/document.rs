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
    pub(crate) fn new(values: Values, defaults: Values) -> Self {
        Self {
            values,
            defaults,
            unsaved: BTreeMap::new(),
            drafts: BTreeMap::new(),
        }
    }

    pub(crate) fn value(&self, key: &Key) -> Option<&Value> {
        self.values.get(key).or_else(|| self.defaults.get(key))
    }

    pub(crate) fn values(&self) -> &Values {
        &self.values
    }

    pub(crate) fn defaults(&self) -> &Values {
        &self.defaults
    }

    pub(crate) fn is_default(&self, row: &Row) -> bool {
        row.default
            .as_ref()
            .is_none_or(|default| self.value(&row.key) == Some(default))
    }

    pub(crate) fn apply(&mut self, key: &Key, value: Value) -> Effect {
        let previous = self.values.get(key).cloned();
        self.unsaved.entry(key.clone()).or_insert(previous);
        self.values.set(key.clone(), value.clone());
        self.drafts.remove(key);
        Effect::Write(key.clone(), value)
    }

    pub(crate) fn acknowledge(&mut self, key: &Key) {
        self.unsaved.remove(key);
    }

    pub(crate) fn fail(&mut self, key: &Key) {
        match self.unsaved.remove(key) {
            Some(Some(previous)) => self.values.set(key.clone(), previous),
            Some(None) => self.values.remove(key),
            None => {},
        }
        self.drafts.remove(key);
    }

    pub(crate) fn remote(&mut self, key: &Key, value: Value) {
        if !self.unsaved.contains_key(key) {
            self.values.set(key.clone(), value);
        }
    }

    pub(crate) fn text(&self, row: &Row) -> String {
        match self.drafts.get(&row.key) {
            Some(draft) => draft.clone(),
            None => row.control.text_of(self.value(&row.key)),
        }
    }

    pub(crate) fn draft(&self, key: &Key) -> Option<&str> {
        self.drafts.get(key).map(String::as_str)
    }

    pub(crate) fn edit(&mut self, key: &Key, text: &str) {
        self.drafts.insert(key.clone(), text.to_owned());
    }

    pub(crate) fn revert(&mut self, key: &Key) {
        self.drafts.remove(key);
    }

    pub(crate) fn is_dirty(&self, row: &Row) -> bool {
        self.drafts
            .get(&row.key)
            .is_some_and(|draft| *draft != row.control.text_of(self.value(&row.key)))
    }

    pub(crate) fn has_unsaved(&self) -> bool {
        !self.unsaved.is_empty()
    }
}

#[cfg(test)]
#[path = "document_tests.rs"]
mod tests;
