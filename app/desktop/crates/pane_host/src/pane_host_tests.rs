use std::rc::Rc;

use model::PaneInstance;
use protocol::{
    CommandId, DaemonRequest, HostId, KeyChord, PaneState, Params,
    RelayEndpoint, RelayScope, Responder,
    decode_shell_frame,
};
use serde_json::json;
use webview::{Bounds, Call, MockPlatform, Visibility, WebViewKind};

use super::*;
use crate::multiplicity::Multiplicity;

pub(crate) struct NoDaemon;

impl DaemonTransport for NoDaemon {
    fn request(&self, _request: DaemonRequest, respond: Responder) {
        respond(Err(protocol::Error::Timeout));
    }

    fn relay_endpoint(&self, _scope: &RelayScope) -> protocol::Result<RelayEndpoint> {
        Err(protocol::Error::Timeout)
    }
}

pub(crate) struct Fixture {
    pub(crate) host: PaneHost,
    pub(crate) mock: Rc<MockPlatform>,
}

const WINDOW: ParentWindow = ParentWindow::X11(1);

pub(crate) fn pane_type(id: &str, lifecycle: PaneLifecycle) -> PaneType {
    PaneType {
        id: PaneTypeId::new(id).unwrap(),
        title: id.into(),
        entry: "x/index.html".into(),
        lifecycle,
        multiplicity: Multiplicity::Many,
    }
}

pub(crate) fn fixture() -> Fixture {
    let mock = Rc::new(MockPlatform::default());
    let config = PaneHostConfig {
        scheme: "crowbar".into(),
        host: HostId::new("local").unwrap(),
        max_idle_slots: 1,
    };
    let webviews = WebViewHost::with_platform(Rc::<MockPlatform>::clone(&mock));
    let mut host = PaneHost::new(config, webviews, Arc::new(NoDaemon), OriginTable::default());
    host.registry_mut()
        .register(pane_type("core.chat", PaneLifecycle::Parkable))
        .unwrap();
    host.registry_mut()
        .register(pane_type("core.term", PaneLifecycle::KeepAlive))
        .unwrap();
    host.registry_mut()
        .register(pane_type("core.scratch", PaneLifecycle::DestroyOnHide))
        .unwrap();
    host.registry_mut()
        .register(pane_type("ext.git.review", PaneLifecycle::Parkable))
        .unwrap();
    Fixture { host, mock }
}

pub(crate) fn instance(id: &str, type_id: &str) -> PaneInstance {
    PaneInstance {
        pane_id: PaneId::new(id).unwrap(),
        type_id: PaneTypeId::new(type_id).unwrap(),
        params: Params::default().with("threadId", id).unwrap(),
        state: PaneState::new(json!({"from": id})).unwrap(),
    }
}

pub(crate) fn bounds() -> Bounds {
    Bounds {
        x: 10.0,
        y: 20.0,
        width: 300.0,
        height: 200.0,
    }
}

pub(crate) fn snapshot(panes: &[PaneInstance], visible: &[&str], focused: Option<&str>) -> LayoutSnapshot {
    LayoutSnapshot {
        parent: WINDOW,
        panes: panes.to_vec(),
        visible: visible
            .iter()
            .map(|id| VisiblePane {
                pane: PaneId::new(*id).unwrap(),
                bounds: bounds(),
            })
            .collect(),
        focused: focused.map(|id| PaneId::new(id).unwrap()),
    }
}

pub(crate) fn frames_sent(mock: &MockPlatform) -> Vec<ShellFrame> {
    let mut frames = Vec::new();
    for call in mock.calls() {
        let Call::Eval { script, .. } = call else {
            continue;
        };
        let literal = script
            .split_once("__crowbarReceive(")
            .and_then(|(_, rest)| rest.strip_suffix(')'));
        let text: String = serde_json::from_str(literal.unwrap()).unwrap();
        frames.push(decode_shell_frame(&text).unwrap());
    }
    frames
}

pub(crate) fn lifecycle_events(mock: &MockPlatform) -> Vec<Lifecycle> {
    frames_sent(mock)
        .into_iter()
        .filter_map(|frame| match frame {
            ShellFrame::Lifecycle { event, .. } => Some(event),
            _ => None,
        })
        .collect()
}

pub(crate) fn register(host: &mut PaneHost, pane: &str, type_id: &str) -> Vec<HostEvent> {
    let text = encode(&protocol::PaneFrame::Register {
        type_id: PaneTypeId::new(type_id).unwrap(),
        sdk: protocol::SDK_VERSION,
    })
    .unwrap();
    host.handle_ipc(IpcMessage {
        pane: PaneId::new(pane).unwrap(),
        text,
    })
    .unwrap()
}

pub(crate) fn shown(mock: &MockPlatform) -> usize {
    mock.calls()
        .iter()
        .filter(|call| {
            matches!(
                call,
                Call::SetVisibility {
                    visibility: Visibility::Shown,
                    ..
                }
            )
        })
        .count()
}

#[test]
fn a_visible_pane_gets_a_hidden_then_shown_web_view_at_its_entry_url() {
    let mut f = fixture();
    let chat = instance("p1", "core.chat");
    f.host
        .sync(&snapshot(&[chat], &["p1"], Some("p1")))
        .unwrap();
    let calls = f.mock.calls();
    assert!(
        matches!(&calls[0], Call::Create { url, kind: WebViewKind::Core, visibility: Visibility::Hidden, .. } if url == "crowbar://local/panes/core/x/index.html"),
        "{calls:?}"
    );
    assert_eq!(shown(&f.mock), 1, "shown once placed");
    assert_eq!(
        f.host.display(&PaneId::new("p1").unwrap()),
        PaneDisplay::Web,
        "display"
    );
}

#[test]
fn register_mounts_with_params_and_state_then_activates_the_focused_pane() {
    let mut f = fixture();
    f.host
        .sync(&snapshot(
            &[instance("p1", "core.chat")],
            &["p1"],
            Some("p1"),
        ))
        .unwrap();
    assert!(
        lifecycle_events(&f.mock).is_empty(),
        "nothing is sent before the page registers"
    );
    register(&mut f.host, "p1", "core.chat");
    let frames = frames_sent(&f.mock);
    assert!(
        matches!(&frames[0], ShellFrame::Lifecycle { event: Lifecycle::Mount, params, state, .. }
            if params.get("threadId") == Some("p1") && state.as_value() == &json!({"from": "p1"})),
        "{frames:?}"
    );
    assert_eq!(
        lifecycle_events(&f.mock),
        [Lifecycle::Mount, Lifecycle::Activate],
        "mount then activate"
    );
}

#[test]
fn focus_moves_activation_between_panes() {
    let mut f = fixture();
    let panes = [instance("p1", "core.chat"), instance("p2", "core.chat")];
    f.host
        .sync(&snapshot(&panes, &["p1", "p2"], Some("p1")))
        .unwrap();
    register(&mut f.host, "p1", "core.chat");
    register(&mut f.host, "p2", "core.chat");
    f.host
        .sync(&snapshot(&panes, &["p1", "p2"], Some("p2")))
        .unwrap();
    let events = lifecycle_events(&f.mock);
    assert_eq!(
        events,
        [
            Lifecycle::Mount,
            Lifecycle::Activate,
            Lifecycle::Mount,
            Lifecycle::Deactivate,
            Lifecycle::Activate
        ],
        "p1 mounted+active, p2 mounted, then focus moved"
    );
}

#[test]
fn a_parkable_pane_gives_its_web_view_to_the_next_pane() {
    let mut f = fixture();
    let panes = [instance("a", "core.chat"), instance("b", "core.chat")];
    f.host.sync(&snapshot(&panes, &["a"], Some("a"))).unwrap();
    register(&mut f.host, "a", "core.chat");
    f.host.sync(&snapshot(&panes, &["b"], Some("b"))).unwrap();
    assert_eq!(
        f.host.web_view_count(),
        1,
        "the slot is reused, not duplicated"
    );
    let creates = f
        .mock
        .calls()
        .iter()
        .filter(|c| matches!(c, Call::Create { .. }))
        .count();
    assert_eq!(creates, 1, "one native web view for two tabs");
    assert!(
        f.mock.calls().iter().any(|c| matches!(c, Call::Navigate { url, .. } if url == "crowbar://local/panes/core/x/index.html")),
        "navigated into the existing view"
    );
    assert!(
        lifecycle_events(&f.mock).contains(&Lifecycle::Park),
        "a was parked"
    );
}

#[test]
fn a_parked_pane_is_restored_from_its_state() {
    let mut f = fixture();
    let panes = [instance("a", "core.chat"), instance("b", "core.chat")];
    f.host.sync(&snapshot(&panes, &["a"], Some("a"))).unwrap();
    register(&mut f.host, "a", "core.chat");
    f.host.sync(&snapshot(&panes, &["b"], Some("b"))).unwrap();
    register(&mut f.host, "b", "core.chat");
    f.host.sync(&snapshot(&panes, &["a"], Some("a"))).unwrap();
    register(&mut f.host, "a", "core.chat");
    let mounts: Vec<ShellFrame> = frames_sent(&f.mock)
        .into_iter()
        .filter(|f| {
            matches!(
                f,
                ShellFrame::Lifecycle {
                    event: Lifecycle::Mount,
                    ..
                }
            )
        })
        .collect();
    assert!(
        matches!(&mounts[2], ShellFrame::Lifecycle { pane_id, state, .. } if pane_id.as_str() == "a" && state.as_value() == &json!({"from": "a"})),
        "a restored from its state"
    );
}

#[test]
fn keep_alive_panes_are_hidden_not_parked() {
    let mut f = fixture();
    let panes = [instance("t", "core.term"), instance("c", "core.chat")];
    f.host.sync(&snapshot(&panes, &["t"], Some("t"))).unwrap();
    register(&mut f.host, "t", "core.term");
    f.host.sync(&snapshot(&panes, &["c"], Some("c"))).unwrap();
    let events = lifecycle_events(&f.mock);
    assert_eq!(
        events,
        [Lifecycle::Mount, Lifecycle::Activate, Lifecycle::Deactivate],
        "deactivated, never parked"
    );
    assert_eq!(
        f.host.web_view_count(),
        2,
        "its web view stays alive beside the chat's"
    );
    assert_eq!(
        f.host.display(&PaneId::new("t").unwrap()),
        PaneDisplay::Empty,
        "nothing drawn for it"
    );
}

#[test]
fn destroy_on_hide_drops_state_and_remounts_empty() {
    let mut f = fixture();
    let panes = [instance("s", "core.scratch")];
    f.host.sync(&snapshot(&panes, &["s"], Some("s"))).unwrap();
    register(&mut f.host, "s", "core.scratch");
    let events = f.host.sync(&snapshot(&panes, &[], None)).unwrap();
    assert_eq!(
        events,
        [HostEvent::StateDiscarded {
            pane: PaneId::new("s").unwrap()
        }],
        "shell is told"
    );
    assert!(
        lifecycle_events(&f.mock).contains(&Lifecycle::Destroy),
        "destroy sent"
    );
    f.host.sync(&snapshot(&panes, &["s"], Some("s"))).unwrap();
    register(&mut f.host, "s", "core.scratch");
    let last_mount = frames_sent(&f.mock).into_iter().rev().find(|f| {
        matches!(
            f,
            ShellFrame::Lifecycle {
                event: Lifecycle::Mount,
                ..
            }
        )
    });
    assert!(
        matches!(last_mount, Some(ShellFrame::Lifecycle { state, .. }) if state == PaneState::default()),
        "empty state"
    );
}

#[test]
fn an_overlay_hides_web_views_without_telling_any_pane() {
    let mut f = fixture();
    let panes = [instance("p1", "core.chat")];
    f.host.sync(&snapshot(&panes, &["p1"], Some("p1"))).unwrap();
    register(&mut f.host, "p1", "core.chat");
    let before = frames_sent(&f.mock).len();
    f.host.set_overlay(Overlay::Active).unwrap();
    assert!(
        matches!(
            f.mock.calls().last(),
            Some(Call::SetVisibility {
                visibility: Visibility::Hidden,
                ..
            })
        ),
        "hidden"
    );
    f.host.sync(&snapshot(&panes, &["p1"], Some("p1"))).unwrap();
    assert!(
        matches!(
            f.mock.calls().last(),
            Some(Call::SetVisibility {
                visibility: Visibility::Hidden,
                ..
            })
        ),
        "stays hidden while the overlay lasts"
    );
    f.host.set_overlay(Overlay::Inactive).unwrap();
    assert!(
        matches!(
            f.mock.calls().last(),
            Some(Call::SetVisibility {
                visibility: Visibility::Shown,
                ..
            })
        ),
        "shown again"
    );
    assert_eq!(frames_sent(&f.mock).len(), before, "no frame was sent");
}

#[test]
fn a_pane_of_an_unregistered_type_shows_a_placeholder_and_keeps_its_state() {
    let mut f = fixture();
    let orphan = instance("o", "ext.gone.thing");
    f.host
        .sync(&snapshot(std::slice::from_ref(&orphan), &["o"], Some("o")))
        .unwrap();
    assert!(f.mock.calls().is_empty(), "no web view for a missing type");
    assert_eq!(
        f.host.display(&orphan.pane_id),
        PaneDisplay::Placeholder {
            type_id: orphan.type_id.clone()
        },
        "placeholder"
    );
    f.host.sync(&snapshot(&[orphan], &[], None)).unwrap();
}

#[test]
fn removing_a_pane_from_the_layout_destroys_it_and_frees_its_view() {
    let mut f = fixture();
    let panes = [instance("p1", "core.chat")];
    f.host.sync(&snapshot(&panes, &["p1"], Some("p1"))).unwrap();
    register(&mut f.host, "p1", "core.chat");
    f.host.sync(&snapshot(&[], &[], None)).unwrap();
    assert!(
        lifecycle_events(&f.mock).contains(&Lifecycle::Destroy),
        "destroy sent"
    );
    assert_eq!(
        f.host.display(&PaneId::new("p1").unwrap()),
        PaneDisplay::Empty,
        "forgotten"
    );
}

#[test]
fn keybindings_reach_mounted_panes_now_and_later() {
    let mut f = fixture();
    let panes = [instance("p1", "core.chat")];
    f.host.sync(&snapshot(&panes, &["p1"], Some("p1"))).unwrap();
    let binding = KeyBinding::new(
        KeyChord::new("mod+k").unwrap(),
        CommandId::new("palette.open").unwrap(),
    );
    f.host.set_keybindings(vec![binding.clone()]).unwrap();
    assert!(
        frames_sent(&f.mock).is_empty(),
        "not mounted yet, nothing sent"
    );
    register(&mut f.host, "p1", "core.chat");
    assert!(
        frames_sent(&f.mock).contains(&ShellFrame::Keybindings {
            bindings: vec![binding]
        }),
        "sent on mount"
    );
}

#[test]
fn send_to_a_pane_without_a_web_view_is_a_typed_error() {
    let f = fixture();
    let frame = ShellFrame::Keybindings {
        bindings: Vec::new(),
    };
    assert!(
        matches!(
            f.host.send(&PaneId::new("nope").unwrap(), &frame),
            Err(Error::NotShown(_))
        ),
        "no view"
    );
}
