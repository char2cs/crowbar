use crate::error::Result;
use crate::parent_window::ParentWindow;
use crate::web_view_config::WebViewConfig;
use crate::web_view_id::WebViewId;

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
