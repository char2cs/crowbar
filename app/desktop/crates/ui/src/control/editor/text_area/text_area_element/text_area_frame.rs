//! What a text area paints in one frame.

use gpui::{PaintQuad, Pixels, Point};

use crate::control::editor::text_area::text_area_layout::TextAreaLayout;

pub(crate) struct TextAreaFrame {
    pub(crate) layout: TextAreaLayout,
    pub(crate) origin: Point<Pixels>,
    pub(crate) scroll: Pixels,
    pub(crate) selection: Vec<PaintQuad>,
    pub(crate) caret: Option<PaintQuad>,
}
