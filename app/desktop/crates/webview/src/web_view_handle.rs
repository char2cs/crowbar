use std::rc::Rc;

use crate::bounds::Bounds;
use crate::error::Result;
use crate::platform::Platform;
use crate::visibility::Visibility;
use crate::web_view_id::WebViewId;

/// The JavaScript function a page defines to receive messages from [`WebViewHandle::post_message`].
pub const RECEIVER: &str = "__crowbarReceive";

/// A live web view. Dropping the handle destroys the view.
pub struct WebViewHandle {
    platform: Rc<dyn Platform>,
    id: WebViewId,
    destroyed: bool,
}

impl std::fmt::Debug for WebViewHandle {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("WebViewHandle")
            .field("id", &self.id)
            .finish_non_exhaustive()
    }
}

impl WebViewHandle {
    pub(crate) fn new(platform: Rc<dyn Platform>, id: WebViewId) -> Self {
        Self {
            platform,
            id,
            destroyed: false,
        }
    }

    /// The view's id, also carried by every [`ProtocolRequest`](crate::ProtocolRequest) it makes.
    #[must_use]
    pub fn id(&self) -> WebViewId {
        self.id
    }

    /// Moves and resizes the view.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn set_bounds(&self, bounds: Bounds) -> Result<()> {
        self.platform.set_bounds(self.id, bounds)
    }

    /// Draws the view.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn show(&self) -> Result<()> {
        self.platform.set_visibility(self.id, Visibility::Shown)
    }

    /// Stops drawing the view; the page stays loaded and is not told.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn hide(&self) -> Result<()> {
        self.platform.set_visibility(self.id, Visibility::Hidden)
    }

    /// Runs a script in the page.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn eval(&self, script: &str) -> Result<()> {
        self.platform.eval(self.id, script)
    }

    /// Loads another page into the view, keeping its native view and web process.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn navigate(&self, url: &str) -> Result<()> {
        self.platform.navigate(self.id, url)
    }

    /// Gives keyboard focus to the page.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn focus(&self) -> Result<()> {
        self.platform.focus(self.id)
    }

    /// Takes keyboard focus back to the parent window.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn focus_parent(&self) -> Result<()> {
        self.platform.focus_parent(self.id)
    }

    /// Delivers a text message to the page by calling its [`RECEIVER`] function.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn post_message(&self, text: &str) -> Result<()> {
        self.eval(&receive_script(text))
    }

    /// Destroys the view now and reports failures, which dropping would only log.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn destroy(mut self) -> Result<()> {
        self.destroyed = true;
        self.platform.destroy(self.id)
    }
}

impl Drop for WebViewHandle {
    fn drop(&mut self) {
        if self.destroyed {
            return;
        }
        if let Err(error) = self.platform.destroy(self.id) {
            tracing::warn!(%error, view = %self.id, "destroying a web view on drop failed");
        }
    }
}

fn receive_script(text: &str) -> String {
    let literal = serde_json::to_string(text).unwrap_or_else(|_| String::from("\"\""));
    format!("window.{RECEIVER}&&window.{RECEIVER}({literal})")
}

#[cfg(test)]
#[path = "web_view_handle_tests.rs"]
mod tests;
