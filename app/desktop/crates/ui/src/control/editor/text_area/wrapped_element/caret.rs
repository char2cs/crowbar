//! Where the caret is and how far the text area is scrolled.

use gpui::Pixels;

pub(crate) trait Caret {
    fn cursor(&self) -> usize;
    fn scroll(&self) -> Pixels;
    fn revealed(&self) -> usize;
}
