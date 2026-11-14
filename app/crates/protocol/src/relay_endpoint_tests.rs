use super::*;

#[test]
fn debug_output_never_holds_the_token() {
    let endpoint = RelayEndpoint::new("ws://127.0.0.1:1", "s3cret-token");
    for text in [
        format!("{endpoint:?}"),
        format!("{endpoint:#?}"),
        format!("{:?}", Some(&endpoint)),
    ] {
        assert!(!text.contains("s3cret-token"), "{text}");
        assert!(text.contains("ws://127.0.0.1:1"), "{text}");
    }
}

#[test]
fn serializes_for_the_page_with_the_token() {
    let endpoint = RelayEndpoint::new("ws://127.0.0.1:1", "t");
    assert_eq!(
        serde_json::to_string(&endpoint).unwrap(),
        r#"{"url":"ws://127.0.0.1:1","token":"t"}"#,
        "wire shape"
    );
    assert_eq!(endpoint.url(), "ws://127.0.0.1:1", "url");
    assert_eq!(endpoint.token(), "t", "token");
}
