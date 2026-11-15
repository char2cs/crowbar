use std::collections::BTreeMap;

use serde::{Deserialize, Deserializer, Serialize};
use serde_json::Value;

use crate::error::{Error, Result};
use crate::setting_key::SettingKey;

/// Most settings one `settings` frame holds.
pub const MAX_SETTING_VALUES: usize = 128;

/// The version of the `settings` frame's value vocabulary; a pane ignores a frame it does not
/// know the version of. It rises only when a key changes meaning, never when a key is added.
pub const SETTINGS_FRAME_VERSION: u32 = 1;

/// The settings a pane follows: key (`wordWrap`) to its value. A key that is absent has its
/// default; the shell sends the whole set again whenever any value changes. Keys are valid
/// [`SettingKey`]s and there are at most [`MAX_SETTING_VALUES`] of them, also on the wire.
#[derive(Debug, Clone, Default, PartialEq, Serialize)]
#[serde(transparent)]
pub struct SettingValues(BTreeMap<SettingKey, Value>);

impl SettingValues {
    /// Adds or replaces one value.
    ///
    /// # Errors
    /// An [`Error`] when the key is not a valid [`SettingKey`] or the set would pass
    /// [`MAX_SETTING_VALUES`] values.
    pub fn with(
        mut self,
        key: impl AsRef<str>,
        value: Value,
    ) -> Result<Self> {
        let key = SettingKey::new(key.as_ref())?;
        if self.0.len() >= MAX_SETTING_VALUES && !self.0.contains_key(&key) {
            return Err(Error::too_many_entries("settings", MAX_SETTING_VALUES));
        }
        self.0.insert(key, value);
        Ok(self)
    }

    /// Looks up one value.
    #[must_use]
    pub fn get(
        &self,
        key: &str,
    ) -> Option<&Value> {
        SettingKey::new(key)
            .ok()
            .and_then(|key| self.0.get(&key))
    }

    /// The number of values.
    #[must_use]
    pub fn len(&self) -> usize {
        self.0.len()
    }

    /// Whether there are no values.
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }
}

impl<'de> Deserialize<'de> for SettingValues {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> std::result::Result<Self, D::Error> {
        BTreeMap::<String, Value>::deserialize(deserializer)?
            .into_iter()
            .try_fold(Self::default(), |values, (key, value)| {
                values.with(key, value)
            })
            .map_err(serde::de::Error::custom)
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn builds_serializes_and_looks_up_values() {
        let values = SettingValues::default()
            .with("wordWrap", json!(true))
            .unwrap()
            .with("hiddenFilePatterns", json!(["*.log"]))
            .unwrap();
        assert_eq!(
            serde_json::to_string(&values).unwrap(),
            r#"{"hiddenFilePatterns":["*.log"],"wordWrap":true}"#,
            "shape"
        );
        assert_eq!(values.get("wordWrap"), Some(&json!(true)), "lookup");
        assert_eq!(values.get("nope"), None, "absent");
        assert_eq!((values.len(), values.is_empty()), (2, false), "size");
        assert!(SettingValues::default().is_empty(), "empty");
    }

    #[test]
    fn keys_and_count_are_bounded_also_on_the_wire() {
        for key in ["", "a b", "a/b"] {
            assert!(
                SettingValues::default().with(key, json!(1)).is_err(),
                "{key:?}"
            );
        }
        let wire: serde_json::Map<String, Value> = (0..=MAX_SETTING_VALUES)
            .map(|index| (format!("k{index}"), json!(index)))
            .collect();
        assert!(
            serde_json::from_value::<SettingValues>(wire.into()).is_err(),
            "too many on the wire"
        );
        assert!(
            serde_json::from_str::<SettingValues>(r#"{"a b":1}"#).is_err(),
            "bad key on the wire"
        );
    }
}
