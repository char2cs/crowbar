//! What the canvas hands to a paint pass: its bounds and the frame prepaint produced.

use gpui::{Bounds, Pixels};

use crate::terminal_view::paint::frame::Frame;

pub(crate) struct Request {
    pub(crate) bounds: Bounds<Pixels>,
    pub(crate) frame: Option<Frame>,
}
