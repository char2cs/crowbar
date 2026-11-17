use crate::browsing_data::BrowsingData;
use crate::page_control::PageControl;
use crate::view_capture::ViewCapture;
use crate::view_focus::ViewFocus;
use crate::view_layout::ViewLayout;
use crate::view_lifecycle::ViewLifecycle;

/// The operations a platform backend offers, as the small capabilities they are made of. One
/// implementation per platform; a recording implementation exists for tests of crates built on
/// top (feature `mock`).
///
/// Backends live on the thread that created them (the GPUI main thread), hence no `Send`.
pub trait Platform:
    ViewLifecycle + ViewLayout + PageControl + ViewFocus + ViewCapture + BrowsingData
{
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;
    use std::sync::Arc;

    use super::*;
    use crate::bounds::Bounds;
    use crate::mock_platform::MockPlatform;
    use crate::parent_window::ParentWindow;
    use crate::visibility::Visibility;
    use crate::web_view_config::WebViewConfig;
    use crate::web_view_id::WebViewId;
    use crate::web_view_kind::WebViewKind;

    fn rect(
        x: f64,
        y: f64,
        width: f64,
        height: f64,
    ) -> Bounds {
        Bounds {
            x,
            y,
            width,
            height,
        }
    }

    #[test]
    fn a_backend_is_driven_through_the_trait_object() {
        let mock = Rc::new(MockPlatform::default());
        let platform: Rc<dyn Platform> = Rc::<MockPlatform>::clone(&mock);
        let config = WebViewConfig {
            url: "about:blank".into(),
            bounds: rect(0.0, 0.0, 1.0, 1.0),
            kind: WebViewKind::Core,
            visibility: Visibility::Hidden,
            background: None,
            initialization_script: None,
            on_ipc: Arc::new(|_| {}),
            on_page: None,
        };
        platform
            .create(WebViewId(1), ParentWindow::X11(1), config)
            .unwrap();
        platform.destroy(WebViewId(1)).unwrap();
        assert!(mock.live_views().is_empty(), "destroyed through the trait");
    }
}
