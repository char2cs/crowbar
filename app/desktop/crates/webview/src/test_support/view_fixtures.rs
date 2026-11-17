use std::rc::Rc;
use std::sync::Arc;

use crate::bounds::Bounds;
use crate::mock_platform::MockPlatform;
use crate::parent_window::ParentWindow;
use crate::view_lifecycle::ViewLifecycle;
use crate::visibility::Visibility;
use crate::web_view_config::WebViewConfig;
use crate::web_view_handle::WebViewHandle;
use crate::web_view_id::WebViewId;
use crate::web_view_kind::WebViewKind;

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
