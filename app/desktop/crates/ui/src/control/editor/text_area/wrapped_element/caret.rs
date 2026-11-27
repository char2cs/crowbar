use gpui::Pixels;

pub(crate) trait Caret {
    fn cursor(&self) -> usize;
    fn scroll(&self) -> Pixels;
    fn revealed(&self) -> usize;
}
