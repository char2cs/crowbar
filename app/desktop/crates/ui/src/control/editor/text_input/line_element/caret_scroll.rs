//! What a single-line field keeps to scroll its text so the caret stays in view.

use gpui::Pixels;

pub(crate) const CARET_WIDTH: f32 = 1.0;

pub(crate) trait CaretScroll {
    fn caret(&self) -> usize;
    fn scrolled(&self) -> Pixels;
}
