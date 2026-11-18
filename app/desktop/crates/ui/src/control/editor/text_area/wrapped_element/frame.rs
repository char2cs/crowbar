//! What a text area paints in one frame.

use gpui::{PaintQuad, Pixels, Point};

use crate::control::editor::text_area::layout::Layout;

pub(crate) struct Frame {
    pub(crate) layout: Layout,
    pub(crate) origin: Point<Pixels>,
    pub(crate) scroll: Pixels,
    pub(crate) selection: Vec<PaintQuad>,
    pub(crate) caret: Option<PaintQuad>,
}
