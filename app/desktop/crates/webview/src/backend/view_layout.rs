use crate::error::Result;
use crate::view::Bounds;
use crate::view::Color;
use crate::view::Visibility;
use crate::view::WebViewId;

/// Where a web view is, whether it is drawn, and what it shows before its page paints.
pub trait ViewLayout {
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

    /// Changes the colour a view shows where its page has not painted.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn set_background(
        &self,
        id: WebViewId,
        color: Color,
    ) -> Result<()>;
}
