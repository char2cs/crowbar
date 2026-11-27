use crate::frame::separator::resize_drag::ResizeDrag;

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Resizing {
    pub drag: ResizeDrag,
    pub width: f32,
}
