use crate::error::Result;
use crate::view::ParentWindow;
use crate::view::WebViewConfig;
use crate::view::WebViewId;

/// Creating and destroying web views.
pub trait ViewLifecycle {
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

    /// Destroys a web view and frees its resources.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn destroy(
        &self,
        id: WebViewId,
    ) -> Result<()>;
}
