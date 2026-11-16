//! The visible window onto scrolling content.

use gpui::Pixels;

pub(crate) struct Viewport {
    pub(crate) height: Pixels,
    pub(crate) content: Pixels,
}
