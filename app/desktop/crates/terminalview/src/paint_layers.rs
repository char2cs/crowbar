//! The frame being painted with the colours it is painted in.

use crate::frame::Frame;
use crate::paint_colors::PaintColors;

#[derive(Clone, Copy)]
pub(crate) struct PaintLayers<'a> {
    pub(crate) frame: &'a Frame,
    pub(crate) colors: &'a PaintColors,
}
