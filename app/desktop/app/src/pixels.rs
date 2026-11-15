use gpui::{Bounds, Pixels};

/// A GPUI rectangle as the web view layer's logical rectangle.
pub(crate) fn to_bounds(bounds: Bounds<Pixels>) -> webview::Bounds {
    webview::Bounds {
        x: f64::from(f32::from(bounds.origin.x)),
        y: f64::from(f32::from(bounds.origin.y)),
        width: f64::from(f32::from(bounds.size.width)),
        height: f64::from(f32::from(bounds.size.height)),
    }
}

#[cfg(test)]
#[path = "pixels_tests.rs"]
mod tests;
