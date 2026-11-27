use crate::error::Result;
use crate::view::Bounds;
use crate::view::Corners;
use crate::view::Id;

/// Cutting parts out of a web view, so the window under it shows there.
pub trait ViewCutout {
    /// Cuts `hole`, in logical pixels from the view's top-left corner, out of a web view; `None`
    /// makes the view whole again. A backend that cannot cut holes leaves the view whole.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn set_cutout(
        &self,
        _id: Id,
        _hole: Option<Bounds>,
    ) -> Result<()> {
        Ok(())
    }

    /// Rounds a web view's corners, so the pane frame around it shows there. A backend that
    /// cannot round them leaves them square.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the view is unknown or the backend has stopped.
    fn set_corners(
        &self,
        _id: Id,
        _corners: Corners,
    ) -> Result<()> {
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Whole;

    impl ViewCutout for Whole {}

    #[test]
    fn a_backend_that_cannot_cut_holes_leaves_the_view_whole() {
        assert!(Whole.set_cutout(Id(1), None).is_ok(), "nothing to cut");
    }
}
