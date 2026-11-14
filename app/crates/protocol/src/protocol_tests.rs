use super::*;

#[test]
fn rejects_missing_or_foreign_sdk_version() {
    for text in [
        r#"{"kind":"pane.setTitle","title":"t"}"#,
        r#"{"sdk":2,"kind":"pane.setTitle","title":"t"}"#,
        r#"{"sdk":"1","kind":"pane.setTitle","title":"t"}"#,
        "[]",
    ] {
        assert!(
            matches!(decode_pane_frame(text), Err(Error::UnsupportedSdk { .. })),
            "{text}"
        );
    }
}

#[test]
fn rejects_garbage_and_unknown_kinds() {
    assert!(
        matches!(decode_pane_frame("nope"), Err(Error::Json(_))),
        "garbage"
    );
    assert!(
        matches!(
            decode_pane_frame(r#"{"sdk":1,"kind":"pane.explode"}"#),
            Err(Error::Json(_))
        ),
        "unknown kind"
    );
}

#[test]
fn rejects_oversized_text_before_parsing() {
    let text = " ".repeat(MAX_FRAME_BYTES + 1);
    assert!(
        matches!(decode_shell_frame(&text), Err(Error::FrameTooLarge { .. })),
        "oversized"
    );
}

#[test]
fn encode_refuses_oversized_frames() {
    let frame = PaneFrame::Request {
        id: crate::RequestId(1),
        method: "m".into(),
        params: Value::String("x".repeat(MAX_FRAME_BYTES)),
    };
    assert!(
        matches!(encode(&frame), Err(Error::FrameTooLarge { .. })),
        "oversized"
    );
}
