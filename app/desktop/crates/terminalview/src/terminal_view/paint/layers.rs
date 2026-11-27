use crate::terminal_view::paint::colors::Colors;
use crate::terminal_view::paint::frame::Frame;

#[derive(Clone, Copy)]
pub(crate) struct Layers<'a> {
    pub(crate) frame: &'a Frame,
    pub(crate) colors: &'a Colors,
}
