//! A set of setting values by key.

mod change;
mod choice;
mod error;
mod key;
mod settings_backend;
mod settings_follow;
mod settings_read;
mod settings_write;
mod snapshot;
mod subscription;
mod value;

pub use change::Change;
pub use choice::Choice;
pub use error::Error;
pub use key::Key;
pub use settings_backend::SettingsBackend;
pub use settings_follow::{Listener, SettingsFollow};
pub use settings_read::SettingsRead;
pub use settings_write::SettingsWrite;
pub use snapshot::Snapshot;
pub use subscription::Subscription;
pub use value::Value;

use std::collections::BTreeMap;

/// The values the backend currently holds; a key that is absent has its default.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct Values(BTreeMap<Key, Value>);

impl Values {
    /// The value of `key`, if one is stored.
    pub fn get(
        &self,
        key: &Key,
    ) -> Option<&Value> {
        self.0.get(key)
    }

    /// Stores `value` under `key`.
    pub fn set(
        &mut self,
        key: Key,
        value: Value,
    ) {
        self.0.insert(key, value);
    }

    /// `self` with `value` stored under `key`.
    #[must_use]
    pub fn with(
        mut self,
        key: impl Into<Key>,
        value: impl Into<Value>,
    ) -> Self {
        self.set(key.into(), value.into());
        self
    }

    /// Forgets `key`, so it reads as its default again.
    pub fn remove(
        &mut self,
        key: &Key,
    ) {
        self.0.remove(key);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn set_get_and_remove_round_trip() {
        let key = Key::from("tabSize");
        let mut values = Values::default().with("tabSize", 4.0);
        assert_eq!(values.get(&key), Some(&Value::Number(4.0)));
        values.set(key.clone(), Value::Number(8.0));
        assert_eq!(values.get(&key), Some(&Value::Number(8.0)));
        values.remove(&key);
        assert_eq!(values.get(&key), None);
    }
}
