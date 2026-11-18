//! A set of setting values by key.

mod backend;
mod change;
mod choice;
mod error;
mod follow;
mod key;
mod read;
mod snapshot;
mod subscription;
mod value;
mod write;

pub use backend::Backend;
pub use change::Change;
pub use choice::Choice;
pub use error::Error;
pub use follow::{Follow, Listener};
pub use key::Key;
pub use read::Read;
pub use snapshot::Snapshot;
pub use subscription::Subscription;
pub use value::Value;
pub use write::Write;

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
