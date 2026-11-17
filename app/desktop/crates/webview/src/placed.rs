use wry::Rect;

use crate::visibility::Visibility;

use crate::error::Result;

pub(crate) trait Placed {
    fn set_visibility(
        &self,
        visibility: Visibility,
    ) -> Result<()>;

    fn set_bounds(
        &self,
        bounds: Rect,
    ) -> Result<()>;
}
