mod page;

use std::rc::Rc;

use crate::backend::Platform;
use crate::error::Result;
use crate::view::Bounds;
use crate::view::Color;
use crate::view::Id;
use crate::view::Visibility;

pub(crate) const RECEIVER: &str = "__crowbarReceive";

/// A live web view. Dropping the handle destroys the view.
pub struct Handle {
    pub(crate) platform: Rc<dyn Platform>,
    pub(crate) id: Id,
    destroyed: bool,
}

impl std::fmt::Debug for Handle {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("Handle")
            .field("id", &self.id)
            .finish_non_exhaustive()
    }
}

impl Handle {
    pub(crate) fn new(
        platform: Rc<dyn Platform>,
        id: Id,
    ) -> Self {
        Self {
            platform,
            id,
            destroyed: false,
        }
    }

    /// The view's id, also carried by every [`ProtocolRequest`](crate::ProtocolRequest) it makes.
    #[must_use]
    pub fn id(&self) -> Id {
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

    /// Cuts `hole`, in logical pixels from the view's top-left corner, out of the view so the
    /// window under it shows there and takes the pointer; `None` makes the view whole again.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn set_cutout(
        &self,
        hole: Option<Bounds>,
    ) -> Result<()> {
        self.platform.set_cutout(self.id, hole)
    }

    /// Draws the view, stops drawing it while the page stays loaded and is not told, or covers
    /// it while the shell draws over it.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn set_visibility(
        &self,
        visibility: Visibility,
    ) -> Result<()> {
        self.platform.set_visibility(self.id, visibility)
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

    /// Gives keyboard focus to the page, wherever the pointer is.
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

impl Drop for Handle {
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
    fn showing_hiding_and_covering_only_change_visibility() {
        for visibility in [Visibility::Hidden, Visibility::Shown, Visibility::Covered] {
            let (mock, view) = handle();
            view.set_visibility(visibility).unwrap();
            let calls = mock.calls();
            let changed = Call::SetVisibility {
                id: view.id(),
                visibility,
            };
            assert_eq!(calls.last(), Some(&changed), "{visibility:?}");
            assert_eq!(calls.len(), 2, "{visibility:?}: create plus one change");
        }
    }

    #[test]
    fn dropping_destroys_exactly_once() {
        let (mock, view) = handle();
        drop(view);
        assert_eq!(mock.live_views(), Vec::<Id>::new(), "destroyed on drop");
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
