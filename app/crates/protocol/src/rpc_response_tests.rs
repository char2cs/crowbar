use serde_json::{Value, json};

use super::*;
use crate::display_text::DisplayText;
use crate::error_code::ErrorCode;
use crate::rpc_error::RpcError;

#[test]
fn a_null_result_survives_a_round_trip_as_a_result() {
    let response = RpcResponse::new(RequestId::new(1), RpcOutcome::Ok(Value::Null));
    let text = serde_json::to_string(&response).unwrap();
    assert_eq!(text, r#"{"id":1,"result":null}"#, "null is written");
    assert_eq!(
        serde_json::from_str::<RpcResponse>(&text).unwrap(),
        response,
        "and read back"
    );
}

#[test]
fn an_error_round_trips() {
    let error = RpcError::new(ErrorCode::Rejected, DisplayText::new("no").unwrap());
    let response = RpcResponse::new(RequestId::new(2), RpcOutcome::Err(error));
    let text = serde_json::to_string(&response).unwrap();
    assert_eq!(
        text, r#"{"id":2,"error":{"code":"Rejected","message":"no"}}"#,
        "shape"
    );
    assert_eq!(
        serde_json::from_str::<RpcResponse>(&text).unwrap(),
        response,
        "round trip"
    );
    assert_eq!(response.id(), RequestId::new(2), "id");
    assert!(matches!(response.outcome(), RpcOutcome::Err(_)), "outcome");
}

#[test]
fn both_or_neither_is_refused_on_the_wire() {
    for wire in [
        json!({"id": 1}),
        json!({"id": 1, "result": 1, "error": {"code": "Timeout", "message": ""}}),
        json!({"id": 1, "result": null, "error": {"code": "Timeout", "message": ""}}),
    ] {
        assert!(
            serde_json::from_value::<RpcResponse>(wire.clone()).is_err(),
            "{wire}"
        );
    }
}
