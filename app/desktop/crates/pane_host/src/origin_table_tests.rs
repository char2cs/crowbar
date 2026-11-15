use protocol::{ExtensionId, PaneOrigin};
use webview::{Bounds, ParentWindow, Visibility, WebViewConfig, WebViewHost, WebViewKind};

use super::*;

fn view_id() -> WebViewId {
    let host = WebViewHost::with_platform(std::rc::Rc::new(webview::MockPlatform::default()));
    let config = WebViewConfig {
        url: String::new(),
        bounds: Bounds {
            x: 0.0,
            y: 0.0,
            width: 1.0,
            height: 1.0,
        },
        kind: WebViewKind::Core,
        visibility: Visibility::Hidden,
        initialization_script: None,
        on_ipc: Arc::new(|_| {}),
    };
    host.create_in(ParentWindow::X11(1), config).unwrap().id()
}

#[test]
fn set_replace_and_remove() {
    let table = OriginTable::default();
    let id = view_id();
    assert_eq!(table.get(id), None, "unknown view");
    table.set(
        id,
        PaneOrigin::Core,
    );
    table.set(
        id,
        PaneOrigin::Extension(ExtensionId::new("git").unwrap()),
    );
    assert_eq!(
        table.get(id),
        Some(PaneOrigin::Extension(ExtensionId::new("git").unwrap())),
        "replaced"
    );
    table.remove(id);
    assert_eq!(table.get(id), None, "removed");
}

#[test]
fn clones_share_the_same_entries() {
    let table = OriginTable::default();
    let copy = table.clone();
    let id = view_id();
    copy.set(
        id,
        PaneOrigin::Core,
    );
    assert!(table.get(id).is_some(), "visible through the other clone");
}
