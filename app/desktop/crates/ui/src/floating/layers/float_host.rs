use raw_window_handle::RawWindowHandle;

use crate::floating::layers::float_region::FloatRegion;

pub trait FloatHost {
    fn cover(
        &self,
        float: RawWindowHandle,
        owner: RawWindowHandle,
    );

    fn wake(
        &self,
        float: RawWindowHandle,
    );

    fn shape(
        &self,
        float: RawWindowHandle,
        seen: &[FloatRegion],
        hit: &[FloatRegion],
    );
}
