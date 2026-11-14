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
        deserializer: D,
    ) -> std::result::Result<Self, D::Error> {
        let value = Value::deserialize(deserializer)?;
        Self::new(value).map_err(serde::de::Error::custom)
    }
}

#[cfg(test)]
#[path = "pane_state_tests.rs"]
mod tests;
