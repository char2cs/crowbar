//! The frame being painted with the colours it is painted in.

use crate::terminal_view::paint::frame::Frame;
use crate::terminal_view::paint::paint_colors::PaintColors;

#[derive(Clone, Copy)]
pub(crate) struct PaintLayers<'a> {
    pub(crate) frame: &'a Frame,
    pub(crate) colors: &'a PaintColors,
}
