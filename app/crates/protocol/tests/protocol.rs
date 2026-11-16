//! Public-API tests: every frame survives encoding, and wire shapes stay stable.

use protocol::{
    AttachmentId, AttachmentType, CommandId, DisplayText, ErrorCode, EventName, ExtensionId,
    ExtensionStatus, KeyBinding, KeyChord, Lifecycle, MethodName, PaneFrame, PaneId, PaneState,
    PaneTitle, PaneTypeId, Params, RequestId, RpcError, RpcOutcome, RpcResponse, SDK_VERSION,
    SETTINGS_FRAME_VERSION, SdkVersion, SettingKey, SettingValues, ShellFrame, SpaceId, StatusKind,
    ThemeTokens, decode_pane_frame, decode_shell_frame, encode,
};
use serde_json::{Value, json};

fn assert_pane_frames_round_trip(frames: &[PaneFrame]) -> protocol::Result<()> {
    for frame in frames {
        let text = encode(frame)?;
        assert_eq!(&decode_pane_frame(&text)?, frame, "{text}");
    }
    Ok(())
}

fn assert_shell_frames_round_trip(frames: &[ShellFrame]) -> protocol::Result<()> {
    for frame in frames {
        let text = encode(frame)?;
        assert_eq!(&decode_shell_frame(&text)?, frame, "{text}");
    }
    Ok(())
}

#[test]
fn every_pane_frame_round_trips() {
    let type_id = PaneTypeId::new("ext.git.review").unwrap();
    let cases = [
        PaneFrame::Register {
            type_id: type_id.clone(),
            sdk: SDK_VERSION,
        },
        PaneFrame::Open {
            type_id,
            params: Params::default().with("threadId", "t").unwrap(),
        },
        PaneFrame::SetTitle {
            title: PaneTitle::new("x").unwrap(),
        },
        PaneFrame::SetState {
            state: PaneState::new(json!({"a": [1, 2]})).unwrap(),
        },
        PaneFrame::Request {
            id: RequestId::new(3),
            method: MethodName::new("review.diff").unwrap(),
            params: json!({"a": 1}),
        },
        PaneFrame::Subscribe {
            id: RequestId::new(4),
            event: EventName::new("review.updated").unwrap(),
        },
        PaneFrame::Unsubscribe {
            id: RequestId::new(4),
        },
        PaneFrame::RunCommand {
            command: CommandId::new("ext.git.openReview").unwrap(),
        },
    ];
    assert_pane_frames_round_trip(&cases).unwrap();
}

#[test]
fn the_sdk_frames_for_settings_composer_and_attachments_round_trip() {
    let key = SettingKey::new("diff.context").unwrap();
    let cases = [
        PaneFrame::SettingsGet {
            id: RequestId::new(5),
            key: key.clone(),
        },
        PaneFrame::SettingsSubscribe {
            id: RequestId::new(6),
            key,
        },
        PaneFrame::SettingsSet {
            version: SETTINGS_FRAME_VERSION,
            key: SettingKey::new("showMinimap").unwrap(),
            value: json!(true),
        },
        PaneFrame::ComposerAttach {
            attachment_type: AttachmentType::new("excalidraw.drawing").unwrap(),
            payload: json!({"elements": []}),
        },
        PaneFrame::ComposerDraft {
            id: RequestId::new(7),
        },
        PaneFrame::AttachmentOpen {
            attachment_id: AttachmentId::new("att-1").unwrap(),
        },
    ];
    assert_pane_frames_round_trip(&cases).unwrap();
}

#[test]
fn every_shell_frame_round_trips() {
    let ext = ExtensionId::new("git").unwrap();
    let cases = [
        ShellFrame::Lifecycle {
            pane_id: PaneId::new("p1").unwrap(),
            event: Lifecycle::Mount,
            params: Params::default().with("spaceId", "s").unwrap(),
            state: PaneState::default(),
        },
        ShellFrame::Response(RpcResponse::new(
            RequestId::new(1),
            RpcOutcome::Ok(json!({"ok": true})),
        )),
        ShellFrame::Response(RpcResponse::new(
            RequestId::new(2),
            RpcOutcome::Err(RpcError::new(
                ErrorCode::Timeout,
                DisplayText::new("slow").unwrap(),
            )),
        )),
        ShellFrame::ExtensionEvent {
            extension_id: ext.clone(),
            space_id: Some(SpaceId::new("s1").unwrap()),
            name: EventName::new("review.updated").unwrap(),
            payload: json!({"n": 1}),
        },
        ShellFrame::ExtensionStatus(ExtensionStatus::new(
            ext,
            StatusKind::Unavailable,
            Some(DisplayText::new("crashed").unwrap()),
        )),
    ];
    assert_shell_frames_round_trip(&cases).unwrap();
}

#[test]
fn the_sdk_push_frames_for_keybindings_settings_and_theme_round_trip() {
    let cases = [
        ShellFrame::Keybindings {
            bindings: vec![KeyBinding::new(
                KeyChord::new("mod+k").unwrap(),
                CommandId::new("palette.open").unwrap(),
            )],
        },
        ShellFrame::SettingChanged {
            id: RequestId::new(6),
            key: SettingKey::new("diff.context").unwrap(),
            value: json!(5),
        },
        ShellFrame::Theme {
            tokens: ThemeTokens::default().with("color-bg", "#111").unwrap(),
        },
        ShellFrame::Settings {
            version: SETTINGS_FRAME_VERSION,
            values: SettingValues::default()
                .with("wordWrap", json!(true))
                .unwrap(),
        },
    ];
    assert_shell_frames_round_trip(&cases).unwrap();
}

#[test]
fn a_successful_null_result_is_not_an_absent_result() {
    let frame = ShellFrame::Response(RpcResponse::new(
        RequestId::new(9),
        RpcOutcome::Ok(Value::Null),
    ));
    let text = encode(&frame).unwrap();
    assert_eq!(
        text, r#"{"sdk":1,"kind":"response","id":9,"result":null}"#,
        "shape"
    );
    assert_eq!(decode_shell_frame(&text).unwrap(), frame, "round trip");
    for both_or_neither in [
        r#"{"sdk":1,"kind":"response","id":9}"#,
        r#"{"sdk":1,"kind":"response","id":9,"result":1,"error":{"code":"Timeout","message":""}}"#,
    ] {
        assert!(
            decode_shell_frame(both_or_neither).is_err(),
            "{both_or_neither}"
        );
    }
}

#[test]
fn wire_shapes_are_camel_case_with_dotted_kinds() {
    let status = ShellFrame::ExtensionStatus(ExtensionStatus::new(
        ExtensionId::new("git").unwrap(),
        StatusKind::Back,
        None,
    ));
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
    let register = PaneFrame::Register {
        type_id: PaneTypeId::new("core.chat").unwrap(),
        sdk: SdkVersion::new(1, 0),
    };
    assert_eq!(
        encode(&register).unwrap(),
        r#"{"sdk":1,"kind":"pane.register","typeId":"core.chat","sdkVersion":"1.0"}"#,
        "register"
    );
}

#[test]
fn the_settings_frame_is_versioned_and_carries_the_whole_set() {
    let frame = ShellFrame::Settings {
        version: SETTINGS_FRAME_VERSION,
        values: SettingValues::default().with("tabSize", json!(4)).unwrap(),
    };
    assert_eq!(
        encode(&frame).unwrap(),
        r#"{"sdk":1,"kind":"settings","version":1,"values":{"tabSize":4}}"#,
        "shape"
    );
    assert!(
        decode_shell_frame(r#"{"sdk":1,"kind":"settings","version":1,"values":{"a b":1}}"#)
            .is_err(),
        "a bad key is refused"
    );
}

#[test]
fn lifecycle_names_are_lower_camel_case() {
    for (event, name) in [
        (Lifecycle::Mount, "mount"),
        (Lifecycle::Activate, "activate"),
        (Lifecycle::Deactivate, "deactivate"),
        (Lifecycle::Park, "park"),
        (Lifecycle::Destroy, "destroy"),
    ] {
        assert_eq!(
            serde_json::to_string(&event).unwrap(),
            format!("\"{name}\""),
            "{event:?}"
        );
    }
}

#[test]
fn oversized_state_in_a_frame_is_rejected() {
    let big = "x".repeat(protocol::MAX_STATE_BYTES);
    let text = format!(r#"{{"sdk":1,"kind":"pane.setState","state":"{big}"}}"#);
    assert!(decode_pane_frame(&text).is_err(), "state over the cap");
}

#[test]
fn unbounded_or_malformed_strings_in_pane_frames_are_rejected() {
    let long = "x".repeat(300);
    for text in [
        format!(r#"{{"sdk":1,"kind":"pane.setTitle","title":"{long}"}}"#),
        r#"{"sdk":1,"kind":"pane.setTitle","title":"a\nb"}"#.to_owned(),
        r#"{"sdk":1,"kind":"request","id":1,"method":"a b","params":null}"#.to_owned(),
        r#"{"sdk":1,"kind":"subscribe","id":1,"event":""}"#.to_owned(),
        r#"{"sdk":1,"kind":"command.run","command":"x/y"}"#.to_owned(),
        r#"{"sdk":1,"kind":"pane.open","typeId":"ext.my_ext.x","params":{}}"#.to_owned(),
        r#"{"sdk":1,"kind":"pane.open","typeId":"core.chat","params":{"k":"a b"}}"#.to_owned(),
    ] {
        assert!(decode_pane_frame(&text).is_err(), "{text}");
    }
}

#[test]
fn a_pane_cannot_claim_an_identity() {
    let text = r#"{"sdk":1,"kind":"pane.setTitle","title":"t","paneId":"other"}"#;
    assert_eq!(
        decode_pane_frame(text).unwrap(),
        PaneFrame::SetTitle {
            title: PaneTitle::new("t").unwrap()
        },
        "the claimed id is ignored"
    );
}
