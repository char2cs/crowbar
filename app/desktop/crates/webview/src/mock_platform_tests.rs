use std::sync::{Arc, Mutex};

use super::*;
use crate::web_view_kind::WebViewKind;

fn config(on_ipc: IpcHandler) -> WebViewConfig {
    WebViewConfig {
        url: "crowbar://local/".into(),
        bounds: Bounds {
            x: 0.0,
            y: 0.0,
            width: 10.0,
            height: 10.0,
        },
        kind: WebViewKind::Core,
        visibility: Visibility::Shown,
        initialization_script: None,
        on_ipc,
    }
}

#[test]
fn records_the_life_of_a_view() {
    let mock = MockPlatform::default();
    let id = WebViewId(1);
    mock.create(id, ParentWindow::X11(9), config(Arc::new(|_| {})))
        .unwrap();
    mock.set_visibility(id, Visibility::Hidden).unwrap();
    mock.destroy(id).unwrap();
    let kinds: Vec<&str> = mock
        .calls()
        .iter()
        .map(|call| match call {
            Call::Create { .. } => "create",
            Call::SetVisibility { .. } => "visibility",
            Call::Destroy { .. } => "destroy",
            _ => "other",
        })
        .collect();
    assert_eq!(kinds, ["create", "visibility", "destroy"], "order");
    assert!(mock.live_views().is_empty(), "nothing alive");
}

#[test]
fn unknown_views_are_errors() {
    let mock = MockPlatform::default();
    let id = WebViewId(5);
    assert!(
        matches!(mock.eval(id, "1"), Err(Error::UnknownWebView(_))),
        "eval"
    );
    assert!(
        matches!(
            mock.set_bounds(
                id,
                Bounds {
                    x: 0.0,
                    y: 0.0,
                    width: 1.0,
                    height: 1.0
                }
            ),
            Err(Error::UnknownWebView(_))
        ),
        "bounds"
    );
    assert!(
        matches!(mock.destroy(id), Err(Error::UnknownWebView(_))),
        "destroy"
    );
    assert!(
        matches!(mock.simulate_ipc(id, "x"), Err(Error::UnknownWebView(_))),
        "ipc"
    );
}

#[test]
fn ipc_reaches_the_handler_of_the_right_view() {
    let mock = MockPlatform::default();
    let seen = Arc::new(Mutex::new(Vec::new()));
    let sink = Arc::clone(&seen);
    mock.create(
        WebViewId(1),
        ParentWindow::X11(1),
        config(Arc::new(move |text| sink.lock().unwrap().push(text))),
    )
    .unwrap();
    mock.create(WebViewId(2), ParentWindow::X11(1), config(Arc::new(|_| {})))
        .unwrap();
    mock.simulate_ipc(WebViewId(1), "hello").unwrap();
    mock.simulate_ipc(WebViewId(2), "ignored").unwrap();
    assert_eq!(
        *seen.lock().unwrap(),
        ["hello"],
        "only view 1 saw its message"
    );
}

#[test]
fn a_create_can_be_made_to_fail_once() {
    let mock = MockPlatform::default();
    mock.fail_next_create();
    assert!(
        mock.create(WebViewId(1), ParentWindow::X11(1), config(Arc::new(|_| {})))
            .is_err(),
        "first fails"
    );
    assert!(
        mock.create(WebViewId(1), ParentWindow::X11(1), config(Arc::new(|_| {})))
            .is_ok(),
        "second works"
    );
}
