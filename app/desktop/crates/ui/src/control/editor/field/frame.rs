//! What a text field paints in one frame.

use gpui::PaintQuad;

use crate::display::TrackedLine;

pub(crate) struct Frame {
    pub(crate) line: TrackedLine,
    pub(crate) selection: Option<PaintQuad>,
    pub(crate) caret: Option<PaintQuad>,
}
