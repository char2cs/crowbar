//! The platform side of the pop-up window, which the binary installs.

pub(super) mod float_region;
pub(super) mod part;
pub(super) mod raw_handle;

use raw_window_handle::RawWindowHandle;

use crate::floating::layers::float_host::float_region::FloatRegion;

/// Places and shapes the stock pop-up window that floats above an owner window. Without one
/// installed, passive portals paint inside their own window.
pub trait FloatHost {
    /// Lays `float` over the content area of `owner`: once it opened, and again whenever the
    /// owner moves or resizes.
    fn cover(
        &self,
        float: RawWindowHandle,
        owner: RawWindowHandle,
    );

    /// Has `float` drawn again even while all of it is cut away, as it is between cards.
    fn wake(
        &self,
        float: RawWindowHandle,
    );

    /// Shows `seen` of `float` and lets the pointer reach it on `hit` only.
    fn shape(
        &self,
        float: RawWindowHandle,
        seen: &[FloatRegion],
        hit: &[FloatRegion],
    );
}
