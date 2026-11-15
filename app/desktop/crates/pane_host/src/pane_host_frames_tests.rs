use protocol::{CommandId, DisplayText, ExtensionId, PaneFrame, PaneState, PaneTitle, encode};
use serde_json::json;
use webview::{Call, Visibility, WebViewKind};

use crate::ipc_message::IpcMessage;
use crate::pane_display::PaneDisplay;
use crate::pane_host::PaneHost;
use super::*;
use crate::pane_host::tests::{
    Fixture, fixture, instance, lifecycle_events, register, snapshot,
};

pub(crate) fn started() -> Fixture {
    let mut f = fixture();
    f.host
        .sync(&snapshot(
            &[instance("p1", "core.chat")],
            &["p1"],
            Some("p1"),
        ))
        .unwrap();
    f
}

fn post(host: &mut PaneHost, frame: &PaneFrame) -> Vec<HostEvent> {
    let text = encode(frame).unwrap();
    host.handle_ipc(IpcMessage {
        pane: PaneId::new("p1").unwrap(),
        text,
    })
    .unwrap()
}

#[test]
fn title_and_state_frames_become_events() {
    let mut f = started();
    let title = PaneTitle::new("Hi").unwrap();
    let titled = post(&mut f.host, &PaneFrame::SetTitle { title: title.clone() });
    let pane = PaneId::new("p1").unwrap();
    assert_eq!(
        titled,
        [HostEvent::TitleChanged {
            pane: pane.clone(),
            title
        }],
        "title"
    );
    let state = PaneState::new(json!({"k": 1})).unwrap();
    let changed = post(&mut f.host, &PaneFrame::SetState { state: state.clone() });
    assert_eq!(changed, [HostEvent::StateChanged { pane, state }], "state");
}

#[test]
fn command_frames_become_events() {
    let mut f = started();
    let command = post(
        &mut f.host,
        &PaneFrame::RunCommand {
            command: CommandId::new("palette.open").unwrap(),
        },
    );
    assert!(
        matches!(&command[..], [HostEvent::CommandRequested { command, .. }] if command.as_str() == "palette.open"),
        "command"
    );
}

#[test]
fn malformed_and_stale_messages_are_contained() {
    let mut f = fixture();
    let panes = [instance("p1", "core.chat"), instance("p2", "core.chat")];
    f.host.sync(&snapshot(&panes, &["p1"], Some("p1"))).unwrap();
    let junk = IpcMessage {
        pane: PaneId::new("p1").unwrap(),
        text: "not json".into(),
    };
    assert!(f.host.handle_ipc(junk).unwrap().is_empty(), "no events");
    assert!(
        matches!(
            f.host.display(&PaneId::new("p1").unwrap()),
            PaneDisplay::Error { .. }
        ),
        "p1 fails"
    );
    let stale = IpcMessage {
        pane: PaneId::new("p2").unwrap(),
        text: "{}".into(),
    };
    assert!(
        f.host.handle_ipc(stale).unwrap().is_empty(),
        "p2 has no web view; ignored"
    );
    assert_eq!(
        f.host.display(&PaneId::new("p2").unwrap()),
        PaneDisplay::Empty,
        "p2 untouched"
    );
}

#[test]
fn a_page_that_registers_the_wrong_type_fails_inside_its_own_frame() {
    let mut f = fixture();
    f.host
        .sync(&snapshot(
            &[instance("p1", "core.chat")],
            &["p1"],
            Some("p1"),
        ))
        .unwrap();
    register(&mut f.host, "p1", "core.term");
    assert!(
        matches!(
            f.host.display(&PaneId::new("p1").unwrap()),
            PaneDisplay::Error { .. }
        ),
        "error display"
    );
    assert!(
        matches!(
            f.mock.calls().last(),
            Some(Call::SetVisibility {
                visibility: Visibility::Hidden,
                ..
            })
        ),
        "web view hidden"
    );
    assert!(lifecycle_events(&f.mock).is_empty(), "never mounted");
}

#[test]
fn extension_panes_get_their_own_web_view_kind_and_react_to_status() {
    let mut f = fixture();
    let review = instance("r", "ext.git.review");
    f.host
        .sync(&snapshot(&[review], &["r"], Some("r")))
        .unwrap();
    let git = ExtensionId::new("git").unwrap();
    assert!(
        matches!(&f.mock.calls()[0], Call::Create { kind: WebViewKind::Extension(e), url, .. } if *e == git && url == "crowbar://local/panes/ext/git/x/index.html"),
        "extension view"
    );
    register(&mut f.host, "r", "ext.git.review");
    let down = ExtensionStatus::new(
        git.clone(),
        StatusKind::Unavailable,
        Some(DisplayText::new("crashed").unwrap()),
    );
    f.host.extension_status(&down).unwrap();
    assert_eq!(
        f.host.display(&PaneId::new("r").unwrap()),
        PaneDisplay::Error {
            message: "crashed".into()
        },
        "error in frame"
    );
    let back = ExtensionStatus::new(git, StatusKind::Back, None);
    f.host.extension_status(&back).unwrap();
    assert_eq!(
        f.host.display(&PaneId::new("r").unwrap()),
        PaneDisplay::Web,
        "recovered"
    );
    let mounts = lifecycle_events(&f.mock)
        .iter()
        .filter(|e| **e == Lifecycle::Mount)
        .count();
    assert_eq!(mounts, 2, "mount re-sent with the saved state");
}


#[test]
fn the_pointer_report_becomes_a_hover_event() {
    let mut f = started();
    let report = IpcMessage {
        pane: PaneId::new("p1").unwrap(),
        text: r#"{"sdk":1,"kind":"shell.hover","hovered":true}"#.into(),
    };
    assert_eq!(
        f.host.handle_ipc(report).unwrap(),
        [HostEvent::HoverChanged {
            pane: PaneId::new("p1").unwrap(),
            hovered: true
        }],
        "hover"
    );
}
