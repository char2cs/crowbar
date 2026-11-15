use std::rc::Rc;
use std::sync::Arc;

use model::TabId;
use pane_host::{OriginTable, PaneHostConfig};
use protocol::{
    DaemonRequest, DaemonTransport, HostId, PaneFrame, PaneState, PaneTitle, RelayEndpoint,
    RelayScope, Responder, encode,
};
use serde_json::json;
use webview::{Bounds, Call, MockPlatform, WebViewHost};

use super::*;
use crate::core_panes::{chat_params, chat_type, register_core_panes};

struct NoDaemon;

impl DaemonTransport for NoDaemon {
    fn request(&self, _request: DaemonRequest, respond: Responder) {
        respond(Err(protocol::Error::Timeout));
    }

    fn relay_endpoint(&self, _scope: &RelayScope) -> protocol::Result<RelayEndpoint> {
        Err(protocol::Error::Timeout)
    }
}

struct Rig {
    shell: ShellState,
    mock: Rc<MockPlatform>,
    dir: tempfile::TempDir,
}

fn host(mock: &Rc<MockPlatform>) -> PaneHost {
    let config = PaneHostConfig {
        scheme: "crowbar".into(),
        host: HostId::new("local").unwrap(),
        max_idle_slots: 1,
    };
    let webviews = WebViewHost::with_platform(Rc::<MockPlatform>::clone(mock));
    let mut host = PaneHost::new(config, webviews, Arc::new(NoDaemon), OriginTable::default());
    register_core_panes(host.registry_mut(), "chat/index.html").unwrap();
    host
}

fn rig() -> Rig {
    let mock = Rc::new(MockPlatform::default());
    let dir = tempfile::tempdir().unwrap();
    let store = LayoutStore::new(dir.path().join("layout.json"));
    let shell = ShellState::new(host(&mock), store).unwrap();
    Rig { shell, mock, dir }
}

fn open_chat(shell: &mut ShellState, thread: &str) {
    shell.open_pane(chat_type().unwrap(), chat_params(thread).unwrap());
}

fn visible(shell: &ShellState) -> Vec<VisiblePane> {
    shell.layout().active_tab().map_or_else(Vec::new, |tab| {
        tab.pane_ids()
            .into_iter()
            .map(|pane| VisiblePane {
                pane,
                bounds: Bounds {
                    x: 294.0,
                    y: 42.0,
                    width: 800.0,
                    height: 600.0,
                },
            })
            .collect()
    })
}

#[test]
fn an_opened_chat_gets_a_web_view_in_the_window() {
    let mut rig = rig();
    open_chat(&mut rig.shell, "t1");
    let visible = visible(&rig.shell);
    rig.shell.sync(ParentWindow::X11(9), visible);
    let calls = rig.mock.calls();
    assert!(
        matches!(&calls[0], Call::Create { url, parent: ParentWindow::X11(9), .. }
            if url == "crowbar://local/panes/core/chat/index.html"),
        "{calls:?}"
    );
}

#[test]
fn pane_state_flows_into_the_layout() {
    let mut rig = rig();
    open_chat(&mut rig.shell, "t1");
    let visible = visible(&rig.shell);
    rig.shell.sync(ParentWindow::X11(9), visible);
    let pane = rig.shell.layout().active_tab().unwrap().pane_ids()[0].clone();
    let state = PaneState::new(json!({"draft": "hi"})).unwrap();
    let text = encode(&PaneFrame::SetState {
        state: state.clone(),
    })
    .unwrap();
    rig.shell.handle_ipc(IpcMessage {
        pane: pane.clone(),
        text,
    });
    let stored = rig.shell.layout().pane(&pane).map(|p| p.state.clone());
    assert_eq!(stored, Some(state), "the model owns the new state");
}

#[test]
fn the_layout_survives_a_restart() {
    let mut rig = rig();
    open_chat(&mut rig.shell, "t1");
    let again = ShellState::new(
        host(&rig.mock),
        LayoutStore::new(rig.dir.path().join("layout.json")),
    )
    .unwrap();
    assert_eq!(
        again.layout(),
        rig.shell.layout(),
        "same layout after loading"
    );
}

#[test]
fn refused_commands_change_nothing_and_do_not_stop_the_rest() {
    let mut rig = rig();
    rig.shell.apply(vec![
        Command::CloseTab { tab: TabId(99) },
        Command::AddTab {
            id: TabId(1),
            title: "ok".into(),
        },
    ]);
    assert_eq!(
        rig.shell.layout().tabs().len(),
        1,
        "the valid command applied"
    );
}

#[test]
fn a_page_title_is_kept_until_its_pane_closes() {
    let mut rig = rig();
    open_chat(&mut rig.shell, "t1");
    let visible = visible(&rig.shell);
    rig.shell.sync(ParentWindow::X11(9), visible);
    let pane = rig.shell.layout().active_tab().unwrap().pane_ids()[0].clone();
    let text = encode(&PaneFrame::SetTitle {
        title: PaneTitle::new("Fix the build").unwrap(),
    })
    .unwrap();
    let outcome = rig.shell.handle_ipc(IpcMessage {
        pane: pane.clone(),
        text,
    });
    assert!(outcome.titles_changed, "the tab strip must refresh");
    assert_eq!(
        rig.shell.titles().get(&pane).map(|t| t.to_string()),
        Some("Fix the build".into()),
        "kept"
    );
    rig.shell
        .apply(vec![Command::ClosePane { pane: pane.clone() }]);
    assert!(
        rig.shell.titles().get(&pane).is_none(),
        "forgotten with the pane"
    );
}
