//! What a text field paints in one frame.

use gpui::{PaintQuad, ShapedLine};

pub(crate) struct TextFrame {
    pub(crate) line: ShapedLine,
    pub(crate) selection: Option<PaintQuad>,
    pub(crate) caret: Option<PaintQuad>,
}
