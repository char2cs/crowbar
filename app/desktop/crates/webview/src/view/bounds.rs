/// A rectangle in logical pixels, relative to the parent window's top-left corner.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Bounds {
    /// Distance from the left edge.
    pub x: f64,
    /// Distance from the top edge.
    pub y: f64,
    /// Width.
    pub width: f64,
    /// Height.
    pub height: f64,
}
