mod page;

use std::rc::Rc;

use crate::backend::Platform;
use crate::error::Result;
use crate::view::Bounds;
use crate::view::Color;
use crate::view::Visibility;
use crate::view::WebViewId;

/// The JavaScript function a page defines to receive messages from [`WebViewHandle::post_message`].
pub const RECEIVER: &str = "__crowbarReceive";

/// A live web view. Dropping the handle destroys the view.
pub struct WebViewHandle {
    pub(crate) platform: Rc<dyn Platform>,
    pub(crate) id: WebViewId,
    destroyed: bool,
}

impl std::fmt::Debug for WebViewHandle {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("WebViewHandle")
            .field("id", &self.id)
            .finish_non_exhaustive()
    }
}

impl WebViewHandle {
    pub(crate) fn new(
        platform: Rc<dyn Platform>,
        id: WebViewId,
    ) -> Self {
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
    pub fn set_bounds(
        &self,
        bounds: Bounds,
    ) -> Result<()> {
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

    /// Changes the colour shown where the page has not painted.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn set_background(
        &self,
        color: Color,
    ) -> Result<()> {
        self.platform.set_background(self.id, color)
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::backend::Call;
    use crate::test_support::view_fixtures::handle;

    #[test]
    fn a_new_background_reaches_the_backend_for_that_view() {
        let (mock, view) = handle();
        let color = Color {
            red: 9,
            green: 8,
            blue: 7,
            alpha: 255,
        };
        view.set_background(color).unwrap();
        assert_eq!(
            mock.calls().last(),
            Some(&Call::SetBackground {
                id: view.id(),
                color
            }),
            "recorded"
        );
    }

    #[test]
    fn show_and_hide_only_change_visibility() {
        type Change = fn(&WebViewHandle);
        let cases: [(&str, Change, Visibility); 2] = [
            ("hide", |view| view.hide().unwrap(), Visibility::Hidden),
            ("show", |view| view.show().unwrap(), Visibility::Shown),
        ];
        for (name, change, expected) in cases {
            assert_change_is_visibility_only(name, change, expected);
        }
    }

    fn assert_change_is_visibility_only(
        name: &str,
        change: fn(&WebViewHandle),
        expected: Visibility,
    ) {
        let (mock, view) = handle();
        change(&view);
        let calls = mock.calls();
        let visibility = Call::SetVisibility {
            id: view.id(),
            visibility: expected,
        };
        assert_eq!(calls.last(), Some(&visibility), "{name}");
        assert_eq!(calls.len(), 2, "{name}: create plus one visibility change");
    }

    #[test]
    fn dropping_destroys_exactly_once() {
        let (mock, view) = handle();
        drop(view);
        assert_eq!(
            mock.live_views(),
            Vec::<WebViewId>::new(),
            "destroyed on drop"
        );
        let (mock, view) = handle();
        view.destroy().unwrap();
        let destroys = mock
            .calls()
            .iter()
            .filter(|c| matches!(c, Call::Destroy { .. }))
            .count();
        assert_eq!(destroys, 1, "explicit destroy is not repeated by drop");
    }
}
