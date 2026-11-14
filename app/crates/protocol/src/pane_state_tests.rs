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
