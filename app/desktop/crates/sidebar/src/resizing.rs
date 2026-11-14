//! A resize in progress.

use crate::resize_drag::ResizeDrag;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct Resizing {
    pub(crate) drag: ResizeDrag,
    pub(crate) width: f32,
}
