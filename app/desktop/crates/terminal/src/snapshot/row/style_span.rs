use crate::snapshot::palette::Rgb;
use crate::snapshot::row::style_flags::StyleFlags;
use crate::snapshot::row::underline::Underline;

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct StyleSpan {
    pub len: usize,
    pub foreground: Rgb,
    pub flags: StyleFlags,
    pub underline: Underline,
}
