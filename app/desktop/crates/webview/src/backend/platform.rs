use crate::backend::browsing_data::BrowsingData;
use crate::backend::desktop_settings::DesktopSettings;
use crate::backend::file_choosing::FileChoosing;
use crate::backend::menu_popping::MenuPopping;
use crate::backend::page_control::PageControl;
use crate::backend::view_capture::ViewCapture;
use crate::backend::view_focus::ViewFocus;
use crate::backend::view_layout::ViewLayout;
use crate::backend::view_lifecycle::ViewLifecycle;

/// The operations a platform backend offers, as the small capabilities they are made of. One
/// implementation per platform; a recording implementation exists for tests of crates built on
/// top (feature `mock`).
///
/// Backends live on the thread that created them (the GPUI main thread), hence no `Send`.
pub trait Platform:
    ViewLifecycle
    + ViewLayout
    + PageControl
    + ViewFocus
    + ViewCapture
    + BrowsingData
    + DesktopSettings
    + FileChoosing
    + MenuPopping
{
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;
    use std::sync::Arc;

    use super::*;
    use crate::backend::mock::MockPlatform;
    use crate::view::Bounds;
    use crate::view::Config;
    use crate::view::Id;
    use crate::view::Kind;
    use crate::view::ParentWindow;
    use crate::view::Visibility;

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
        let config = Config {
            url: "about:blank".into(),
            bounds: rect(0.0, 0.0, 1.0, 1.0),
            kind: Kind::Core,
            visibility: Visibility::Hidden,
            background: None,
            initialization_script: None,
            on_ipc: Arc::new(|_| {}),
            on_page: None,
        };
        platform
            .create(Id(1), ParentWindow::X11(1), config)
            .unwrap();
        platform.destroy(Id(1)).unwrap();
        assert!(mock.live_views().is_empty(), "destroyed through the trait");
    }
}
