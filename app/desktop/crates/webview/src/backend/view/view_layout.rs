use crate::error::Result;
use crate::view::Bounds;
use crate::view::Color;
use crate::view::Id;
use crate::view::Visibility;

pub trait ViewLayout {
    fn set_bounds(
        &self,
        id: Id,
        bounds: Bounds,
    ) -> Result<()>;

    fn set_visibility(
        &self,
        id: Id,
        visibility: Visibility,
    ) -> Result<()>;

    fn set_background(
        &self,
        id: Id,
        color: Color,
    ) -> Result<()>;
}
