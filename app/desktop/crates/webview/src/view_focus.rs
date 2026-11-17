use crate::error::Result;
use crate::web_view_id::WebViewId;

/// Which side holds keyboard focus: the page or the parent window.
pub trait ViewFocus {
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
}
