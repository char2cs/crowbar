use gpui::{Bounds, DevicePixels};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct FloatRegion {
    pub bounds: Bounds<DevicePixels>,
    pub radius: DevicePixels,
}
