use super::*;

#[test]
fn carries_a_code_and_bounded_text() {
    let error = RpcError::new(ErrorCode::Timeout, DisplayText::new("too slow").unwrap());
    assert_eq!(error.code(), ErrorCode::Timeout, "code");
    assert_eq!(error.message().as_str(), "too slow", "message");
    let long = format!(r#"{{"code":"Timeout","message":"{}"}}"#, "x".repeat(2000));
    assert!(
        serde_json::from_str::<RpcError>(&long).is_err(),
        "message is capped on the wire"
    );
}
