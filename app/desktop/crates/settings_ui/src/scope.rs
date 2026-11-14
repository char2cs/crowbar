//! What a [`crate::Condition`] is evaluated against.

use std::collections::BTreeSet;

use crate::key::Key;
use crate::value::Value;
use crate::values::Values;

#[derive(Debug, Clone, Copy)]
pub(crate) struct Scope<'a> {
    pub(crate) values: &'a Values,
    pub(crate) defaults: &'a Values,
    pub(crate) capabilities: &'a BTreeSet<String>,
}

impl Scope<'_> {
    pub(crate) fn lookup(&self, key: &Key) -> Option<&Value> {
        self.values.get(key).or_else(|| self.defaults.get(key))
    }
}

#[cfg(test)]
#[path = "scope_tests.rs"]
mod tests;
