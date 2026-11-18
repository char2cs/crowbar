//! What a text field paints in one frame.

use gpui::{PaintQuad, ShapedLine};

pub(crate) struct Frame {
    pub(crate) line: ShapedLine,
    pub(crate) selection: Option<PaintQuad>,
    pub(crate) caret: Option<PaintQuad>,
}
