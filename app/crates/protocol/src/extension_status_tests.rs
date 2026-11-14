use super::*;

#[test]
fn matches_the_host_protocol_shape() {
    let status = ExtensionStatus::new(
        ExtensionId::new("git").unwrap(),
        StatusKind::Unavailable,
        Some(DisplayText::new("crashed").unwrap()),
    );
    let text = serde_json::to_string(&status).unwrap();
    assert_eq!(
        text, r#"{"extensionId":"git","status":"unavailable","reason":"crashed"}"#,
        "shape"
    );
    assert_eq!(
        serde_json::from_str::<ExtensionStatus>(&text).unwrap(),
        status,
        "round trip"
    );
    assert_eq!(status.extension_id().as_str(), "git", "extension");
    assert_eq!(status.status(), StatusKind::Unavailable, "status");
    assert_eq!(
        status.reason().map(DisplayText::as_str),
        Some("crashed"),
        "reason"
    );
}

#[test]
fn a_running_extension_has_no_reason() {
    let status = ExtensionStatus::new(ExtensionId::new("git").unwrap(), StatusKind::Running, None);
    assert_eq!(
        serde_json::to_string(&status).unwrap(),
        r#"{"extensionId":"git","status":"running"}"#,
        "reason omitted"
    );
}
