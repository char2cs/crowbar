use std::rc::Rc;

use pane_host::OriginTable;
use protocol::{PaneFrame, PaneId, PaneState, encode};
use serde_json::json;
use webview::{Call, MockPlatform};

use super::*;
use crate::loopback_daemon::LoopbackDaemon;

fn state() -> (ShellState, Rc<MockPlatform>) {
    let mock = Rc::new(MockPlatform::default());
    let config = PaneHostConfig { scheme: "crowbar".into(), host: HostId::new("local").unwrap(), max_idle_slots: 1 };
    let host = PaneHost::new(config, WebViewHost::with_platform(mock.clone()), Arc::new(LoopbackDaemon), OriginTable::default());
    (ShellState::with_host(host, "chat/index.html").unwrap(), mock)
}

const CONTENT: Bounds = Bounds { x: 220.0, y: 36.0, width: 800.0, height: 600.0 };

#[test]
fn the_chat_pane_gets_the_content_area() {
    let (mut shell, mock) = state();
    shell.set_content(ParentWindow::X11(9), CONTENT);
    let calls = mock.calls();
    assert!(
        matches!(&calls[0], Call::Create { url, bounds, parent: ParentWindow::X11(9), .. }
            if url == "crowbar://local/panes/core/chat/index.html" && *bounds == CONTENT),
        "{calls:?}"
    );
}

#[test]
fn moving_the_content_area_moves_the_web_view_without_recreating_it() {
    let (mut shell, mock) = state();
    shell.set_content(ParentWindow::X11(9), CONTENT);
    shell.set_content(ParentWindow::X11(9), Bounds { width: 500.0, ..CONTENT });
    let creates = mock.calls().iter().filter(|c| matches!(c, Call::Create { .. })).count();
    assert_eq!(creates, 1, "one web view");
    assert!(matches!(mock.calls().last(), Some(Call::SetVisibility { .. }) | Some(Call::SetBounds { .. })), "re-placed");
}

#[test]
fn pane_state_flows_into_the_layout() {
    let (mut shell, _mock) = state();
    shell.set_content(ParentWindow::X11(9), CONTENT);
    let pane = PaneId::new("chat-1").unwrap();
    let state = PaneState::new(json!({"draft": "hi"})).unwrap();
    let text = encode(&PaneFrame::SetState { state: state.clone() }).unwrap();
    shell.handle_ipc(IpcMessage { pane: pane.clone(), text });
    let stored = shell.layout.layout().tab_of(&pane).and_then(|tab| tab.pane(&pane)).map(|p| p.state.clone());
    assert_eq!(stored, Some(state), "the model owns the new state");
}

#[test]
fn nothing_happens_before_the_content_area_is_known() {
    let (mut shell, mock) = state();
    shell.sync();
    assert!(mock.calls().is_empty(), "no parent window yet");
}
