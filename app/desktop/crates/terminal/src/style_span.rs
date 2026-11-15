//! A run of characters in one text style.

use crate::rgb::Rgb;
use crate::style_flags::StyleFlags;
use crate::underline::Underline;

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
