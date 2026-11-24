use std::rc::Rc;
use std::sync::Arc;

use layout::{Command, Layout, PaneInstance, TabId};
use protocol::{
    HostId, Lifecycle, PaneId, PaneState, PaneTypeId, Params, ShellFrame, decode_shell_frame,
    encode,
};
use serde_json::json;
use webview::{Bounds, Call, MockPlatform, ParentWindow, Visibility};

use crate::catalog::Multiplicity;
use crate::catalog::PaneKind;
use crate::catalog::PaneLifecycle;
use crate::catalog::PaneType;
use crate::pane_host::Keyboard;
use crate::pane_host::LayoutSnapshot;
use crate::pane_host::PaneHost;
use crate::pane_host::PaneKinds;
use crate::pane_host::VisiblePane;
use crate::scheme::OriginTable;
use crate::slot::Config;
use crate::slot::HostEvent;
use crate::slot::IpcMessage;
use crate::test_support::Revocations;

pub(crate) struct Fixture {
    pub(crate) host: PaneHost,
    pub(crate) mock: Rc<MockPlatform>,
    pub(crate) layout: Layout,
    pub(crate) revoked: Arc<Revocations>,
}

impl Fixture {
    pub(crate) fn sync(
        &mut self,
        panes: &[PaneInstance],
        visible: &[&str],
        focused: Option<&str>,
    ) -> Vec<HostEvent> {
        self.sync_with(panes, visible, (focused, Keyboard::Pane))
    }

    pub(crate) fn sync_with(
        &mut self,
        panes: &[PaneInstance],
        visible: &[&str],
        (focused, keyboard): (Option<&str>, Keyboard),
    ) -> Vec<HostEvent> {
        self.layout = layout_of(panes, focused);
        let snapshot = LayoutSnapshot {
            layout: &self.layout,
            parent: WINDOW,
            visible: visible.iter().map(|id| visible_pane(id)).collect(),
            focused: focused.map(|id| PaneId::new(id).unwrap()),
            keyboard,
            kinds: &(),
        };
        synced(&mut self.host, &snapshot)
    }

    pub(crate) fn sync_kinds(
        &mut self,
        panes: &[PaneInstance],
        visible: &[&str],
        kinds: &dyn PaneKinds,
    ) -> Vec<HostEvent> {
        self.layout = layout_of(panes, None);
        let visible = visible.iter().map(|id| visible_pane(id)).collect();
        let focus = self
            .layout
            .showing("")
            .and_then(|tab| tab.focused().cloned());
        let snapshot = LayoutSnapshot::from_layout(&self.layout, WINDOW, visible)
            .with_focus(focus)
            .with_kinds(kinds);
        synced(&mut self.host, &snapshot)
    }

    pub(crate) fn ipc(
        &mut self,
        message: IpcMessage,
    ) -> Vec<HostEvent> {
        self.host.handle_ipc(message, &self.layout).unwrap()
    }
}

fn synced(
    host: &mut PaneHost,
    snapshot: &LayoutSnapshot<'_>,
) -> Vec<HostEvent> {
    let report = host.sync(snapshot);
    assert!(report.failure.is_none(), "{:?}", report.failure);
    report.events
}

fn visible_pane(id: &str) -> VisiblePane {
    VisiblePane {
        pane: PaneId::new(id).unwrap(),
        bounds: bounds(),
    }
}

fn open_pane(
    layout: &mut Layout,
    instance: &PaneInstance,
) {
    layout
        .apply(Command::OpenPane {
            tab: TabId(1),
            instance: instance.clone(),
            placement: None,
        })
        .unwrap();
}

fn focus_pane(
    layout: &mut Layout,
    pane: &str,
) {
    layout
        .apply(Command::FocusPane {
            pane: PaneId::new(pane).unwrap(),
        })
        .unwrap();
}

fn layout_of(
    panes: &[PaneInstance],
    focused: Option<&str>,
) -> Layout {
    let mut layout = Layout::default();
    layout
        .apply(Command::AddTab {
            id: TabId(1),
            title: "t".into(),
            space: layout::SpaceId::default(),
        })
        .unwrap();
    for instance in panes {
        open_pane(&mut layout, instance);
    }
    if let Some(pane) = focused {
        focus_pane(&mut layout, pane);
    }
    layout
}

const WINDOW: ParentWindow = ParentWindow::X11(1);

pub(crate) fn pane_type(
    id: &str,
    lifecycle: PaneLifecycle,
) -> PaneType {
    PaneType {
        id: PaneTypeId::new(id).unwrap(),
        title: id.into(),
        entry: "x/index.html".into(),
        kind: PaneKind::Web,
        lifecycle,
        multiplicity: Multiplicity::Many,
    }
}

pub(crate) fn fixture() -> Fixture {
    let mock = Rc::new(MockPlatform::default());
    let config = Config {
        scheme: "crowbar".into(),
        host: HostId::new("local").unwrap(),
        max_idle_slots: 1,
        page_handler: None,
    };
    let webviews = webview::Host::with_platform(Rc::<MockPlatform>::clone(&mock));
    let revoked = Arc::<Revocations>::default();
    let transport: Arc<dyn protocol::DaemonTransport> = Arc::<Revocations>::clone(&revoked);
    let mut host = PaneHost::new(config, webviews, OriginTable::default(), transport);
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
    Fixture {
        host,
        mock,
        layout: Layout::default(),
        revoked,
    }
}

pub(crate) fn instance(
    id: &str,
    type_id: &str,
) -> PaneInstance {
    PaneInstance {
        pane_id: PaneId::new(id).unwrap(),
        type_id: PaneTypeId::new(type_id).unwrap(),
        params: Params::default().with("threadId", id).unwrap(),
        state: PaneState::new(json!({"from": id})).unwrap(),
    }
}

pub(crate) fn browser_at(
    id: &str,
    url: &str,
) -> PaneInstance {
    PaneInstance {
        state: PaneState::new(json!({ "url": url })).unwrap(),
        ..instance(id, "core.browser")
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

fn frame_of(call: Call) -> Option<ShellFrame> {
    let Call::Eval { script, .. } = call else {
        return None;
    };
    let literal = script
        .split_once("__crowbarReceive(")
        .and_then(|(_, rest)| rest.strip_suffix(')'));
    let text: String = serde_json::from_str(literal.unwrap()).unwrap();
    Some(decode_shell_frame(&text).unwrap())
}

pub(crate) fn frames_sent(mock: &MockPlatform) -> Vec<ShellFrame> {
    mock.calls().into_iter().filter_map(frame_of).collect()
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

pub(crate) fn register(
    f: &mut Fixture,
    pane: &str,
    type_id: &str,
) -> Vec<HostEvent> {
    let text = encode(&protocol::PaneFrame::Register {
        type_id: PaneTypeId::new(type_id).unwrap(),
        sdk: protocol::SDK_VERSION,
    })
    .unwrap();
    f.ipc(IpcMessage {
        pane: PaneId::new(pane).unwrap(),
        text,
    })
}

fn is_shown(call: &Call) -> bool {
    matches!(
        call,
        Call::SetVisibility {
            visibility: Visibility::Shown,
            ..
        }
    )
}

pub(crate) fn shown(mock: &MockPlatform) -> usize {
    mock.calls().iter().filter(|call| is_shown(call)).count()
}

pub(crate) fn blank_config(kind: webview::Kind) -> webview::Config {
    webview::Config {
        url: String::new(),
        bounds: bounds(),
        kind,
        visibility: Visibility::Hidden,
        background: None,
        initialization_script: None,
        on_ipc: Arc::new(|_| {}),
        on_page: None,
    }
}
