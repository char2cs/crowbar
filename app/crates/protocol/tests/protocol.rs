//! Public-API tests: every frame survives encoding, and wire shapes stay stable.

use protocol::{
    ErrorCode, ExtensionId, ExtensionStatus, KeyBinding, Lifecycle, PaneFrame, PaneId, PaneState,
    PaneTypeId, Params, RequestId, RpcError, ShellFrame, SpaceId, StatusKind, decode_pane_frame,
    decode_shell_frame, encode,
};
use serde_json::json;

#[test]
fn every_pane_frame_round_trips() {
    let type_id = PaneTypeId::new("ext.git.review").unwrap();
    let frames = [
        PaneFrame::Register {
            type_id: type_id.clone(),
        },
        PaneFrame::Open {
            type_id,
            params: Params::default().with("threadId", "t"),
        },
        PaneFrame::SetTitle { title: "x".into() },
        PaneFrame::SetState {
            state: PaneState::new(json!({"a": [1, 2]})).unwrap(),
        },
        PaneFrame::Request {
            id: RequestId(3),
            method: "review.diff".into(),
            params: json!({"a": 1}),
        },
        PaneFrame::Subscribe {
            id: RequestId(4),
            event: "review.updated".into(),
        },
        PaneFrame::Unsubscribe { id: RequestId(4) },
        PaneFrame::RunCommand {
            command: "ext.git.openReview".into(),
        },
    ];
    for frame in frames {
        let text = encode(&frame).unwrap();
        assert_eq!(decode_pane_frame(&text).unwrap(), frame, "{text}");
    }
}

#[test]
fn every_shell_frame_round_trips() {
    let frames = [
        ShellFrame::Lifecycle {
            pane_id: PaneId::new("p1").unwrap(),
            event: Lifecycle::Mount,
            params: Params::default().with("spaceId", "s"),
            state: PaneState::default(),
        },
        ShellFrame::Response {
            id: RequestId(1),
            result: Some(json!({"ok": true})),
            error: None,
        },
        ShellFrame::Response {
            id: RequestId(2),
            result: None,
            error: Some(RpcError {
                code: ErrorCode::Timeout,
                message: "slow".into(),
            }),
        },
    ];
    for frame in frames {
        let text = encode(&frame).unwrap();
        assert_eq!(decode_shell_frame(&text).unwrap(), frame, "{text}");
    }
}

#[test]
fn every_extension_frame_round_trips() {
    let ext = ExtensionId::new("git").unwrap();
    let frames = [
        ShellFrame::ExtensionEvent {
            extension_id: ext.clone(),
            space_id: Some(SpaceId::new("s1").unwrap()),
            name: "review.updated".into(),
            payload: json!({"n": 1}),
        },
        ShellFrame::ExtensionStatus(ExtensionStatus {
            extension_id: ext,
            status: StatusKind::Unavailable,
            reason: Some("crashed".into()),
        }),
        ShellFrame::Keybindings {
            bindings: vec![KeyBinding {
                keys: "mod+k".into(),
                command: "palette.open".into(),
            }],
        },
    ];
    for frame in frames {
        let text = encode(&frame).unwrap();
        assert_eq!(decode_shell_frame(&text).unwrap(), frame, "{text}");
    }
}

#[test]
fn wire_shapes_are_camel_case_with_dotted_kinds() {
    let status = ShellFrame::ExtensionStatus(ExtensionStatus {
        extension_id: ExtensionId::new("git").unwrap(),
        status: StatusKind::Back,
        reason: None,
    });
    assert_eq!(
        encode(&status).unwrap(),
        r#"{"sdk":1,"kind":"extension.status","extensionId":"git","status":"back"}"#,
        "status"
    );
    let park = ShellFrame::Lifecycle {
        pane_id: PaneId::new("p1").unwrap(),
        event: Lifecycle::Park,
        params: Params::default(),
        state: PaneState::default(),
    };
    assert_eq!(
        encode(&park).unwrap(),
        r#"{"sdk":1,"kind":"lifecycle","paneId":"p1","event":"park","params":{},"state":null}"#,
        "lifecycle"
    );
}

#[test]
fn oversized_state_in_a_frame_is_rejected() {
    let big = "x".repeat(protocol::MAX_STATE_BYTES);
    let text = format!(r#"{{"sdk":1,"kind":"pane.setState","state":"{big}"}}"#);
    assert!(decode_pane_frame(&text).is_err(), "state over the cap");
}

#[test]
fn a_pane_cannot_claim_an_identity() {
    let text = r#"{"sdk":1,"kind":"pane.setTitle","title":"t","paneId":"other"}"#;
    assert_eq!(
        decode_pane_frame(text).unwrap(),
        PaneFrame::SetTitle { title: "t".into() },
        "the claimed id is ignored"
    );
}
