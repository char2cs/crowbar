use crate::rect::Rect;

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Placement {
    pub bounds: Rect,
    pub maximized: bool,
}
