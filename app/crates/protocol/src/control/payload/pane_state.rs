mod byte_counter;

use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::control::payload::pane_state::byte_counter::ByteCounter;
use crate::error::{Error, Result};

const MAX_STATE_BYTES: usize = 64 * 1024;

#[derive(Debug, Clone, Default, PartialEq, Serialize)]
#[serde(transparent)]
pub struct PaneState(Value);

impl PaneState {
    pub fn new(value: Value) -> Result<Self> {
        let mut counter = ByteCounter(0);
        serde_json::to_writer(&mut counter, &value)?;
        if counter.0 > MAX_STATE_BYTES {
            return Err(Error::state_too_large(counter.0, MAX_STATE_BYTES));
        }
        Ok(Self(value))
    }

    #[must_use]
    pub fn as_value(&self) -> &Value {
        &self.0
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

    const CASES: [(usize, bool); 2] = [(MAX_STATE_BYTES - 2, true), (MAX_STATE_BYTES - 1, false)];

    #[test]
    fn the_cap_is_inclusive_of_the_json_quotes() {
        for (len, accepted) in CASES {
            let result = PaneState::new(string_of_len(len));
            assert_eq!(result.is_ok(), accepted, "len {len}");
        }
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
        assert_eq!(small.as_value(), &json!({"scroll": 10}), "small state");
    }
}
