use gpui::Pixels;

pub(crate) const CARET_WIDTH: f32 = 1.0;

pub(crate) trait CaretScroll {
    fn caret(&self) -> usize;
    fn scrolled(&self) -> Pixels;
}
