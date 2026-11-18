//! A resize in progress.

use crate::resize_drag::ResizeDrag;

/// The separator drag under way and the width it has reached so far.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Resizing {
    /// The pointer drag of the sidebar separator.
    pub drag: ResizeDrag,
    /// The sidebar width at the pointer's latest position.
    pub width: f32,
}
