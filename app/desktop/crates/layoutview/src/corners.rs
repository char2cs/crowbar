/// Four corner radii in logical pixels.
#[derive(Debug, Clone, Copy, PartialEq, Default)]
pub struct Corners {
    /// Top left.
    pub top_left: f32,
    /// Top right.
    pub top_right: f32,
    /// Bottom left.
    pub bottom_left: f32,
    /// Bottom right.
    pub bottom_right: f32,
}
