use gpui::{Pixels, Point};

#[derive(Debug, Clone, PartialEq)]
pub enum Event {
    TitleChanged(String),
    Bell,
    ClipboardWritten(usize),
    Exited(i32),
    OpenUrl {
        url: String,
        position: Point<Pixels>,
    },
    OpenFile {
        path: String,
        line: Option<u32>,
    },
    FontSizeChanged(f32),
}
