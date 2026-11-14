use super::*;

#[test]
fn serializes_as_a_plain_object_in_key_order() {
    let params = Params::default()
        .with("threadId", "t1")
        .with("spaceId", "s1");
    assert_eq!(
        serde_json::to_string(&params).unwrap(),
        r#"{"spaceId":"s1","threadId":"t1"}"#,
        "key order"
    );
    assert_eq!(params.get("spaceId"), Some("s1"), "lookup");
    assert_eq!(params.iter().count(), 2, "iteration");
    assert!(!params.is_empty(), "non-empty");
    assert!(Params::default().is_empty(), "empty");
}

#[test]
fn rejects_non_string_values() {
    assert!(
        serde_json::from_str::<Params>(r#"{"a":1}"#).is_err(),
        "number value"
    );
}
