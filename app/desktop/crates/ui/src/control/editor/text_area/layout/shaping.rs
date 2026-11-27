use gpui::{Pixels, SharedString, TextRun};

pub(crate) struct Shaping<'a> {
    pub(crate) text: SharedString,
    pub(crate) font_size: Pixels,
    pub(crate) runs: &'a [TextRun],
    pub(crate) wrap_width: Option<Pixels>,
    pub(crate) line_height: Pixels,
}
