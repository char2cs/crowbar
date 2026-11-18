use crate::rect::Rect;

/// Where to open the window.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Placement {
    /// The window rectangle, on a connected display.
    pub bounds: Rect,
    /// Open maximized, with `bounds` as the size to restore to.
    pub maximized: bool,
}
