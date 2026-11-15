use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::byte_counter::ByteCounter;
use crate::error::{Error, Result};

/// Largest serialized pane state accepted, in bytes. Larger data belongs in extension data.
pub const MAX_STATE_BYTES: usize = 64 * 1024;

/// Opaque JSON a pane owns and gets back on `mount`. The cap holds for every value of the
/// type, including ones read from the wire or from disk.
#[derive(Debug, Clone, Default, PartialEq, Serialize)]
#[serde(transparent)]
pub struct PaneState(Value);

impl PaneState {
    /// Validates the size of `value` and wraps it.
    ///
    /// # Errors
    /// [`Error::StateTooLarge`] when the serialized value exceeds [`MAX_STATE_BYTES`].
    pub fn new(value: Value) -> Result<Self> {
        let mut counter = ByteCounter(0);
        serde_json::to_writer(&mut counter, &value)?;
        if counter.0 > MAX_STATE_BYTES {
            return Err(Error::StateTooLarge {
                size: counter.0,
                limit: MAX_STATE_BYTES,
            });
        }
        Ok(Self(value))
    }

    /// The wrapped JSON value.
    #[must_use]
    pub fn as_value(&self) -> &Value {
        &self.0
    }

    /// Unwraps the JSON value.
    #[must_use]
    pub fn into_value(self) -> Value {
        self.0
    }
}

impl<'de> Deserialize<'de> for PaneState {
    fn deserialize<D: serde::Deserializer<'de>>(
        deserializer: D
    ) -> std::result::Result<Self, D::Error> {
        let value = Value::deserialize(deserializer)?;
        Self::new(value).map_err(serde::de::Error::custom)
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    fn string_of_len(len: usize) -> Value {
        Value::String("x".repeat(len))
    }

    #[test]
    fn accepts_state_at_the_limit() {
        let state = PaneState::new(string_of_len(MAX_STATE_BYTES - 2)).unwrap();
        assert_eq!(
            state.as_value().as_str().map(str::len),
            Some(MAX_STATE_BYTES - 2),
            "kept"
        );
    }

    #[test]
    fn rejects_state_one_byte_over() {
        let err = PaneState::new(string_of_len(MAX_STATE_BYTES - 1)).unwrap_err();
        assert!(
            matches!(err, Error::StateTooLarge { size, .. } if size == MAX_STATE_BYTES + 1),
            "{err}"
        );
    }

    #[test]
    fn deserialize_enforces_the_cap() {
        let big = serde_json::to_string(&string_of_len(MAX_STATE_BYTES)).unwrap();
        assert!(
            serde_json::from_str::<PaneState>(&big).is_err(),
            "oversized"
        );
        let small: PaneState = serde_json::from_str(r#"{"scroll":10}"#).unwrap();
        assert_eq!(small.into_value(), json!({"scroll": 10}), "small state");
    }

    #[test]
    fn default_state_is_null() {
        assert_eq!(PaneState::default().as_value(), &Value::Null, "null");
    }
}
