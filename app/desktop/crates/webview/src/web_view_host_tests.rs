use std::sync::Arc;

use super::*;
use crate::bounds::Bounds;
use crate::call::Call;
use crate::mock_platform::MockPlatform;
use crate::visibility::Visibility;
use crate::web_view_kind::WebViewKind;

fn config(kind: WebViewKind) -> WebViewConfig {
    WebViewConfig {
        url: "crowbar://local/".into(),
        bounds: Bounds {
            x: 1.0,
            y: 2.0,
            width: 3.0,
            height: 4.0,
        },
        kind,
        visibility: Visibility::Hidden,
        initialization_script: None,
        on_ipc: Arc::new(|_| {}),
    }
}

#[test]
fn every_view_gets_a_fresh_id() {
    let mock = Rc::new(MockPlatform::default());
    let host = WebViewHost::with_platform(mock.clone());
    let a = host
        .create_in(ParentWindow::X11(7), config(WebViewKind::Core))
        .unwrap();
    let b = host
        .create_in(ParentWindow::X11(7), config(WebViewKind::Core))
        .unwrap();
    assert_ne!(a.id(), b.id(), "ids differ");
    assert_eq!(mock.live_views().len(), 2, "both alive");
}

#[test]
fn a_failed_create_registers_nothing() {
    let mock = Rc::new(MockPlatform::default());
    let host = WebViewHost::with_platform(mock.clone());
    mock.fail_next_create();
    assert!(
        host.create_in(ParentWindow::X11(7), config(WebViewKind::Core))
            .is_err(),
        "fails"
    );
    assert!(
        mock.calls().is_empty() && mock.live_views().is_empty(),
        "nothing recorded"
    );
}

#[test]
fn the_config_reaches_the_backend_unchanged() {
    let mock = Rc::new(MockPlatform::default());
    let host = WebViewHost::with_platform(mock.clone());
    let extension = protocol::ExtensionId::new("git").unwrap();
    let _view = host
        .create_in(
            ParentWindow::X11(7),
            config(WebViewKind::Extension(extension.clone())),
        )
        .unwrap();
    assert!(
        matches!(
            mock.calls().first(),
            Some(Call::Create { parent: ParentWindow::X11(7), kind: WebViewKind::Extension(e), visibility: Visibility::Hidden, .. }) if *e == extension
        ),
        "{:?}",
        mock.calls()
    );
}
