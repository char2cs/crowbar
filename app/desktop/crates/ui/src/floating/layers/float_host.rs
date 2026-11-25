//! The platform side of float windows, which the binary installs.

use raw_window_handle::RawWindowHandle;

use crate::floating::layers::float_region::FloatRegion;
use crate::floating::portal::float::Float;

/// Places, shapes and focuses the stock windows that float above an owner window. Without
/// one installed, portals paint inside their own window.
pub trait FloatHost {
    /// Lays `float`, opened as `kind`, over the content area of `owner`: once it opened, and
    /// again whenever the owner moves or resizes.
    fn cover(
        &self,
        float: RawWindowHandle,
        owner: RawWindowHandle,
        kind: Float,
    );

    /// Shows `seen` of `float` and lets the pointer reach it on `hit` only.
    fn shape(
        &self,
        float: RawWindowHandle,
        seen: &[FloatRegion],
        hit: &[FloatRegion],
    );

    /// Gives the keyboard to `float`.
    fn focus(
        &self,
        float: RawWindowHandle,
    );
}
