use std::rc::Rc;
use std::sync::Arc;

use super::*;
use crate::mock_platform::MockPlatform;
use crate::web_view_kind::WebViewKind;

#[test]
fn a_backend_is_driven_through_the_trait_object() {
    let mock = Rc::new(MockPlatform::default());
    let platform: Rc<dyn Platform> = Rc::<MockPlatform>::clone(&mock);
    let config = WebViewConfig {
        url: "about:blank".into(),
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
    platform
        .create(WebViewId(1), ParentWindow::X11(1), config)
        .unwrap();
    platform.destroy(WebViewId(1)).unwrap();
    assert!(mock.live_views().is_empty(), "destroyed through the trait");
}
