//! One card of a pop-up window, in device pixels from the window's top-left corner.

use gpui::{Bounds, DevicePixels};

/// A rounded rectangle a pop-up window shows or lets the pointer reach; see
/// [`crate::FloatHost::shape`].
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct FloatRegion {
    /// Where it is.
    pub bounds: Bounds<DevicePixels>,
    /// Its corner radius.
    pub radius: DevicePixels,
}
