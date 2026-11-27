/// The radii a web view rounds its corners with, in logical pixels.
#[derive(Debug, Clone, Copy, PartialEq, Default)]
pub struct Corners {
    /// Top left.
    pub top_left: f64,
    /// Top right.
    pub top_right: f64,
    /// Bottom right.
    pub bottom_right: f64,
    /// Bottom left.
    pub bottom_left: f64,
}
