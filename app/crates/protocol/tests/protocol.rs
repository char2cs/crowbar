//! Public-API tests: every frame survives encoding, and wire shapes stay stable.

use protocol::{
    CommandId, DisplayText, DocumentPath, EventName, ExtensionId, ExtensionStatus, KeyBinding,
    KeyChord, Lifecycle, MethodName, OpenMode, PaneFrame, PaneId, PaneOpen, PaneState, PaneTitle,
    PaneTypeId, Params, RequestId, SDK_VERSION, SETTINGS_FRAME_VERSION, SdkVersion, SettingKey,
    SettingValues, ShellFrame, SpaceId, StatusKind, ThemeTokens, decode_pane_frame,
    decode_shell_frame, encode,
};
use serde_json::json;

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
        PaneFrame::Open(PaneOpen {
            type_id,
            params: Params::default().with("threadId", "t").unwrap(),
            file: Some(DocumentPath::new("src/a.ts").unwrap()),
            mode: OpenMode::Preview,
        }),
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
        PaneFrame::SetDirty { dirty: true },
    ];
    assert_pane_frames_round_trip(&cases).unwrap();
}

#[test]
fn the_settings_set_frame_round_trips() {
    let cases = [PaneFrame::SettingsSet {
        version: SETTINGS_FRAME_VERSION,
        key: SettingKey::new("showMinimap").unwrap(),
        value: json!(true),
    }];
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
        ShellFrame::ExtensionEvent {
            extension_id: ext.clone(),
            space_id: Some(SpaceId::new("s1").unwrap()),
            name: EventName::new("review.updated").unwrap(),
            payload: json!({"n": 1}),
        },
        ShellFrame::Save,
        ShellFrame::ExtensionStatus(ExtensionStatus::new(
            ext,
            StatusKind::Unavailable,
            Some(DisplayText::new("crashed").unwrap()),
        )),
        ShellFrame::Keybindings {
            bindings: vec![KeyBinding::new(
                KeyChord::new("mod+k").unwrap(),
                CommandId::new("palette.open").unwrap(),
            )],
        },
        ShellFrame::ProtocolError {
            message: DisplayText::new("frame too large").unwrap(),
        },
        ShellFrame::Theme {
            tokens: ThemeTokens::default().with("color-bg", "#111").unwrap(),
        },
        ShellFrame::Group { chat_fills: true },
    ];
    assert_shell_frames_round_trip(&cases).unwrap();
}

#[test]
fn wire_shapes_are_camel_case_with_dotted_kinds() {
    let status = ShellFrame::ExtensionStatus(ExtensionStatus::new(
        ExtensionId::new("git").unwrap(),
        StatusKind::Back,
        None,
    ));
    let park = ShellFrame::Lifecycle {
        pane_id: PaneId::new("p1").unwrap(),
        event: Lifecycle::Park,
        params: Params::default(),
        state: PaneState::default(),
    };
    let register = PaneFrame::Register {
        type_id: PaneTypeId::new("core.chat").unwrap(),
        sdk: SdkVersion::new(1, 0),
    };
    let cases = [
        (
            encode(&status),
            r#"{"sdk":1,"kind":"extension.status","extensionId":"git","status":"back"}"#,
        ),
        (
            encode(&park),
            r#"{"sdk":1,"kind":"lifecycle","paneId":"p1","event":"park","params":{},"state":null}"#,
        ),
        (
            encode(&register),
            r#"{"sdk":1,"kind":"pane.register","typeId":"core.chat","sdkVersion":"1.0"}"#,
        ),
        (
            encode(&ShellFrame::Group { chat_fills: false }),
            r#"{"sdk":1,"kind":"group","chatFills":false}"#,
        ),
    ];
    for (encoded, expected) in cases {
        assert_eq!(encoded.unwrap(), expected, "{expected}");
    }
}

#[test]
fn the_settings_frame_is_versioned_and_carries_the_whole_set() {
    let frame = ShellFrame::Settings {
        version: SETTINGS_FRAME_VERSION,
        values: SettingValues::default().with("tabSize", json!(4)).unwrap(),
    };
    let text = encode(&frame).unwrap();
    assert_eq!(
        text, r#"{"sdk":1,"kind":"settings","version":1,"values":{"tabSize":4}}"#,
        "shape"
    );
    assert_eq!(decode_shell_frame(&text).unwrap(), frame, "round trip");
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
        format!(
            r#"{{"sdk":1,"kind":"pane.setState","state":"{}"}}"#,
            "x".repeat(64 * 1024)
        ),
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

fn editor_open(
    file: Option<&str>,
    mode: OpenMode,
) -> protocol::Result<PaneFrame> {
    Ok(PaneFrame::Open(PaneOpen {
        type_id: PaneTypeId::new("core.editor")?,
        params: Params::default(),
        file: file.map(DocumentPath::new).transpose()?,
        mode,
    }))
}

#[test]
fn a_pane_open_names_a_document_and_whether_it_previews_and_old_pages_still_open_tabs() {
    let old = r#"{"sdk":1,"kind":"pane.open","typeId":"core.editor","params":{}}"#;
    let new = r#"{"sdk":1,"kind":"pane.open","typeId":"core.editor","params":{},"file":"src/a.ts","mode":"preview"}"#;
    let cases = [
        (old, editor_open(None, OpenMode::Kept).unwrap()),
        (
            new,
            editor_open(Some("src/a.ts"), OpenMode::Preview).unwrap(),
        ),
    ];
    for (text, expected) in cases {
        assert_eq!(decode_pane_frame(text).unwrap(), expected, "{text}");
    }
    let climbing =
        r#"{"sdk":1,"kind":"pane.open","typeId":"core.editor","params":{},"file":"../x"}"#;
    assert!(
        decode_pane_frame(climbing).is_err(),
        "no way out of the workspace"
    );
}
