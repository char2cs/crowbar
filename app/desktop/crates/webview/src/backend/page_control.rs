use crate::error::Result;
use crate::view::WebViewId;

/// What runs in a web view's page.
pub trait PageControl {
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
}
