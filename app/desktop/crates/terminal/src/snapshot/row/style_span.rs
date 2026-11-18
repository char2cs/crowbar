//! A run of characters in one text style.

use crate::snapshot::row::style_flags::StyleFlags;
use crate::snapshot::row::underline::Underline;
use crate::snapshot::terminal_palette::Rgb;

/// `len` bytes of a segment's text drawn in one foreground colour and attribute set.
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct StyleSpan {
    /// Length in bytes of the text this span covers.
    pub len: usize,
    /// Glyph colour.
    pub foreground: Rgb,
    /// Weight, slant and strike-through.
    pub flags: StyleFlags,
    /// Underline variant.
    pub underline: Underline,
}
