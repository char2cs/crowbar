use crate::error::Result;
use crate::view::Bounds;
use crate::view::Corners;
use crate::view::Id;

pub trait ViewCutout {
    fn set_cutout(
        &self,
        _id: Id,
        _hole: Option<Bounds>,
    ) -> Result<()> {
        Ok(())
    }

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
