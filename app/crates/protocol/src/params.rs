use std::collections::BTreeMap;

use serde::{Deserialize, Deserializer, Serialize};

use crate::error::{Error, Result};
use crate::string_id::{check_text, is_word_char};

/// Most entries a [`Params`] holds.
pub const MAX_PARAMS: usize = 16;

const MAX_KEY_BYTES: usize = 64;

/// The parameters a pane instance is identified by (`spaceId`, `threadId`, ...).
///
/// Values are ids only: panes fetch their data from the daemon. Keys are alphanumeric words
/// (`-` and `_` allowed) of at most 64 bytes, values follow the id alphabet, and there are at
/// most [`MAX_PARAMS`] entries; every value of the type, including ones read from the wire,
/// holds these limits. The map is ordered so equal parameters always serialize and compare the
/// same way.
#[derive(Debug, Clone, Default, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize)]
#[serde(transparent)]
pub struct Params(BTreeMap<String, String>);

impl Params {
    /// Adds or replaces one entry.
    ///
    /// # Errors
    /// An [`Error`] when the key or value is outside its alphabet or length, or the entry
    /// would be the [`MAX_PARAMS`]th plus one.
    pub fn with(mut self, key: impl Into<String>, value: impl Into<String>) -> Result<Self> {
        let (key, value) = (key.into(), value.into());
        check_entry(&key, &value)?;
        if self.0.len() >= MAX_PARAMS && !self.0.contains_key(&key) {
            return Err(Error::TooManyEntries {
                kind: "params",
                max: MAX_PARAMS,
            });
        }
        self.0.insert(key, value);
        Ok(self)
    }

    /// Looks up one entry.
    #[must_use]
    pub fn get(&self, key: &str) -> Option<&str> {
        self.0.get(key).map(String::as_str)
    }

    /// Whether there are no entries.
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }

    /// Iterates entries in key order.
    pub fn iter(&self) -> impl Iterator<Item = (&str, &str)> {
        self.0.iter().map(|(k, v)| (k.as_str(), v.as_str()))
    }
}

fn check_entry(key: &str, value: &str) -> Result<()> {
    if key.len() > MAX_KEY_BYTES {
        return Err(Error::IdTooLong {
            kind: "param key",
            max: MAX_KEY_BYTES,
        });
    }
    check_text("param key", key, |c| {
        c.is_ascii_alphanumeric() || matches!(c, '-' | '_')
    })?;
    check_text("param value", value, is_word_char)
}

impl<'de> Deserialize<'de> for Params {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> std::result::Result<Self, D::Error> {
        let entries = BTreeMap::<String, String>::deserialize(deserializer)?;
        entries
            .into_iter()
            .try_fold(Self::default(), |params, (key, value)| {
                params.with(key, value)
            })
            .map_err(serde::de::Error::custom)
    }
}

#[cfg(test)]
#[path = "params_tests.rs"]
mod tests;
