mod page;

use std::rc::Rc;

use crate::backend::Platform;
use crate::error::Result;
use crate::view::Bounds;
use crate::view::Color;
use crate::view::Corners;
use crate::view::Id;
use crate::view::Visibility;

pub(crate) const RECEIVER: &str = "__crowbarReceive";

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

    #[must_use]
    pub fn id(&self) -> Id {
        self.id
    }

    pub fn set_bounds(
        &self,
        bounds: Bounds,
    ) -> Result<()> {
        self.platform.set_bounds(self.id, bounds)
    }

    pub fn set_cutout(
        &self,
        hole: Option<Bounds>,
    ) -> Result<()> {
        self.platform.set_cutout(self.id, hole)
    }

    pub fn set_corners(
        &self,
        corners: Corners,
    ) -> Result<()> {
        self.platform.set_corners(self.id, corners)
    }

    pub fn set_visibility(
        &self,
        visibility: Visibility,
    ) -> Result<()> {
        self.platform.set_visibility(self.id, visibility)
    }

    pub fn set_background(
        &self,
        color: Color,
    ) -> Result<()> {
        self.platform.set_background(self.id, color)
    }

    pub fn focus(&self) -> Result<()> {
        self.platform.focus(self.id)
    }

    pub fn focus_parent(&self) -> Result<()> {
        self.platform.focus_parent(self.id)
    }

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
        type Change = fn(&Handle);
        let cases: [(&str, Change, Visibility); 3] = [
            (
                "hide",
                |view| view.set_visibility(Visibility::Hidden).unwrap(),
                Visibility::Hidden,
            ),
            (
                "show",
                |view| view.set_visibility(Visibility::Shown).unwrap(),
                Visibility::Shown,
            ),
            (
                "cover",
                |view| view.set_visibility(Visibility::Covered).unwrap(),
                Visibility::Covered,
            ),
        ];
        for (name, change, expected) in cases {
            assert_change_is_visibility_only(name, change, expected);
        }
    }

    fn assert_change_is_visibility_only(
        name: &str,
        change: fn(&Handle),
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
