use serde_json::{Value, json};

use super::*;

#[test]
fn a_present_null_is_some_and_an_absent_key_is_none() {
    let null: RpcResponseWire = serde_json::from_value(json!({"id": 1, "result": null})).unwrap();
    assert_eq!(null.result, Some(Value::Null), "null is present");
    let absent: RpcResponseWire = serde_json::from_value(json!({"id": 1})).unwrap();
    assert_eq!(absent.result, None, "absent");
}
