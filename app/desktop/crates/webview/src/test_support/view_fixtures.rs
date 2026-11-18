use std::rc::Rc;
use std::sync::Arc;

use crate::backend::MockPlatform;
use crate::backend::ViewLifecycle;
use crate::view::Bounds;
use crate::view::ParentWindow;
use crate::view::Visibility;
use crate::view::WebViewConfig;
use crate::view::WebViewId;
use crate::view::WebViewKind;
use crate::web_view_host::WebViewHandle;

pub(crate) fn handle() -> (Rc<MockPlatform>, WebViewHandle) {
    let mock = Rc::new(MockPlatform::default());
    let bounds = Bounds {
        x: 0.0,
        y: 0.0,
        width: 1.0,
        height: 1.0,
    };
    let config = WebViewConfig {
        url: "x".into(),
        bounds,
        kind: WebViewKind::Core,
        visibility: Visibility::Shown,
        background: None,
        initialization_script: None,
        on_ipc: Arc::new(|_| {}),
        on_page: None,
    };
    mock.create(WebViewId(0), ParentWindow::X11(1), config)
        .unwrap();
    (Rc::clone(&mock), WebViewHandle::new(mock, WebViewId(0)))
}
