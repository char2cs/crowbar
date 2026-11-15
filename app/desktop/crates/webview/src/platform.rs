use crate::bounds::Bounds;
use crate::error::Result;
use crate::parent_window::ParentWindow;
use crate::visibility::Visibility;
use crate::web_view_config::WebViewConfig;
use crate::web_view_id::WebViewId;

/// The operations a platform backend offers. One implementation per platform; a recording
/// implementation exists for tests of crates built on top (feature `mock`).
///
/// Backends live on the thread that created them (the GPUI main thread), hence no `Send`.
pub trait Platform {
    /// Creates a web view as a child of `parent`.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the engine cannot create the view.
    fn create(
        &self,
        id: WebViewId,
        parent: ParentWindow,
        config: WebViewConfig,
    ) -> Result<()>;

    /// Moves and resizes a web view.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn set_bounds(
        &self,
        id: WebViewId,
        bounds: Bounds,
    ) -> Result<()>;

    /// Shows or hides a web view without unloading it.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn set_visibility(
        &self,
        id: WebViewId,
        visibility: Visibility,
    ) -> Result<()>;

    /// Runs a script in the page.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn eval(
        &self,
        id: WebViewId,
        script: &str,
    ) -> Result<()>;

    /// Loads another page into an existing web view.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn navigate(
        &self,
        id: WebViewId,
        url: &str,
    ) -> Result<()>;

    /// Gives keyboard focus to the page.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn focus(
        &self,
        id: WebViewId,
    ) -> Result<()>;

    /// Takes keyboard focus back from the page to the parent window.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn focus_parent(
        &self,
        id: WebViewId,
    ) -> Result<()>;

    /// Destroys a web view and frees its resources.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn destroy(
        &self,
        id: WebViewId,
    ) -> Result<()>;
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;
    use std::sync::Arc;

    use super::*;
    use crate::mock_platform::MockPlatform;
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
