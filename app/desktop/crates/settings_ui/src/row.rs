//! One setting in a section.

use crate::condition::Condition;
use crate::control::Control;
use crate::key::Key;
use crate::value::Value;

/// A label, description and control bound to one setting key.
#[derive(Debug, Clone, PartialEq)]
pub struct Row {
    pub(crate) key: Key,
    pub(crate) label: String,
    pub(crate) description: Option<String>,
    pub(crate) invalid_description: Option<String>,
    pub(crate) control: Control,
    pub(crate) default: Option<Value>,
    pub(crate) when: Option<Condition>,
    pub(crate) keywords: Vec<String>,
}

impl Row {
    /// A row for `key` shown as `label` with `control`.
    pub fn new(key: impl Into<Key>, label: impl Into<String>, control: Control) -> Self {
        Self {
            key: key.into(),
            label: label.into(),
            description: None,
            invalid_description: None,
            control,
            default: None,
            when: None,
            keywords: Vec::new(),
        }
    }

    /// The text under the label.
    #[must_use]
    pub fn describe(mut self, text: impl Into<String>) -> Self {
        self.description = Some(text.into());
        self
    }

    /// The text under the label while the field holds an invalid value.
    #[must_use]
    pub fn describe_invalid(mut self, text: impl Into<String>) -> Self {
        self.invalid_description = Some(text.into());
        self
    }

    /// The default, which also makes the row resettable.
    #[must_use]
    pub fn default_value(mut self, value: impl Into<Value>) -> Self {
        self.default = Some(value.into());
        self
    }

    /// Shows the row only while `condition` holds.
    #[must_use]
    pub fn when(mut self, condition: Condition) -> Self {
        self.when = Some(condition);
        self
    }

    /// Extra search terms.
    #[must_use]
    pub fn keywords(mut self, words: &[&str]) -> Self {
        self.keywords = words.iter().map(|&word| word.to_owned()).collect();
        self
    }

    /// The setting this row edits.
    pub fn key(&self) -> &Key {
        &self.key
    }

    /// The label shown to the user.
    pub fn label(&self) -> &str {
        &self.label
    }
}

#[cfg(test)]
#[path = "row_tests.rs"]
mod tests;
