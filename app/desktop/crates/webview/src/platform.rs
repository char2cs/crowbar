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
    fn create(&self, id: WebViewId, parent: ParentWindow, config: WebViewConfig) -> Result<()>;

    /// Moves and resizes a web view.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn set_bounds(&self, id: WebViewId, bounds: Bounds) -> Result<()>;

    /// Shows or hides a web view without unloading it.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn set_visibility(&self, id: WebViewId, visibility: Visibility) -> Result<()>;

    /// Runs a script in the page.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn eval(&self, id: WebViewId, script: &str) -> Result<()>;

    /// Loads another page into an existing web view.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn navigate(&self, id: WebViewId, url: &str) -> Result<()>;

    /// Gives keyboard focus to the page.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn focus(&self, id: WebViewId) -> Result<()>;

    /// Takes keyboard focus back from the page to the parent window.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn focus_parent(&self, id: WebViewId) -> Result<()>;

    /// Destroys a web view and frees its resources.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn destroy(&self, id: WebViewId) -> Result<()>;
}
