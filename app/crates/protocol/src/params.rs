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
    pub fn with(
        mut self,
        key: impl Into<String>,
        value: impl Into<String>,
    ) -> Result<Self> {
        let (key, value) = (key.into(), value.into());
        check_entry(&key, &value)?;
        if self.0.len() >= MAX_PARAMS && !self.0.contains_key(&key) {
            return Err(Error::too_many_entries("params", MAX_PARAMS));
        }
        self.0.insert(key, value);
        Ok(self)
    }

    /// Looks up one entry.
    #[must_use]
    pub fn get(
        &self,
        key: &str,
    ) -> Option<&str> {
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

fn check_entry(
    key: &str,
    value: &str,
) -> Result<()> {
    if key.len() > MAX_KEY_BYTES {
        return Err(Error::too_long("param key", MAX_KEY_BYTES));
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
mod tests {
    use super::*;

    #[test]
    fn serializes_as_a_plain_object_in_key_order() {
        let params = Params::default()
            .with("threadId", "t1")
            .unwrap()
            .with("spaceId", "s1")
            .unwrap();
        assert_eq!(
            serde_json::to_string(&params).unwrap(),
            r#"{"spaceId":"s1","threadId":"t1"}"#,
            "key order"
        );
        assert_eq!(params.get("spaceId"), Some("s1"), "lookup");
        assert_eq!(params.iter().count(), 2, "iteration");
        assert!(!params.is_empty(), "non-empty");
        assert!(Params::default().is_empty(), "empty");
    }

    #[test]
    fn rejects_non_string_values() {
        assert!(
            serde_json::from_str::<Params>(r#"{"a":1}"#).is_err(),
            "number value"
        );
    }

    #[test]
    fn keys_and_values_stay_inside_their_alphabets() {
        const CASES: [(&str, &str); 6] = [
            ("", "v"),
            ("a b", "v"),
            ("k.k", "v"),
            ("k", ""),
            ("k", "a b"),
            ("k", "a/b"),
        ];
        for (key, value) in CASES {
            assert!(
                Params::default().with(key, value).is_err(),
                "{key:?}={value:?}"
            );
        }
        let long = "k".repeat(65);
        assert!(Params::default().with(long, "v").is_err(), "long key");
        assert!(
            Params::default().with("k", "v".repeat(129)).is_err(),
            "long value"
        );
    }

    #[test]
    fn entry_count_is_capped_on_construction_and_on_the_wire() {
        let mut params = Params::default();
        for index in 0..MAX_PARAMS {
            params = params.with(format!("k{index}"), "v").unwrap();
        }
        assert!(
            params.clone().with("one-too-many", "v").is_err(),
            "over the cap"
        );
        assert!(
            params.with("k0", "other").is_ok(),
            "replacing is not growing"
        );
        let wire: serde_json::Map<String, serde_json::Value> = (0..=MAX_PARAMS)
            .map(|index| (format!("k{index}"), "v".into()))
            .collect();
        assert!(
            serde_json::from_value::<Params>(wire.into()).is_err(),
            "wire object over the cap"
        );
        assert!(
            serde_json::from_str::<Params>(r#"{"bad key":"v"}"#).is_err(),
            "wire key validated"
        );
    }
}
