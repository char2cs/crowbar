const CORNERS: usize = 4;

/// A page's size and the radii of its corners, in device pixels, clockwise from the top left.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Rounded {
    /// The page's width.
    pub width: u16,
    /// The page's height.
    pub height: u16,
    /// Top left, top right, bottom right, bottom left; zero is square.
    pub radii: [u16; CORNERS],
}
