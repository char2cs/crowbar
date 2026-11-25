//! The fixed colour a workspace status glyph is drawn in.

use gpui::{Hsla, rgb};

// SPEC: https://tailwindcss.com/docs/colors (v4 amber/red/green/violet-500, as develop draws them)
const AMBER_500: u32 = 0x00fe_9a00;
const RED_500: u32 = 0x00fb_2c36;
const GREEN_500: u32 = 0x0000_c950;
const VIOLET_500: u32 = 0x008e_51ff;

/// The colour a row glyph keeps on any ground: none for the plain glyphs, which follow the text.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum GlyphTone {
    /// Drawn in the row's text colour.
    Plain,
    /// A branch that needs attention: a placeholder or conflicts.
    Attention,
    /// A deleted branch or a closed pull request.
    Closed,
    /// An open pull request.
    Open,
    /// A merged pull request.
    Merged,
}

impl GlyphTone {
    /// The fixed colour, or none when the glyph follows the text colour.
    #[must_use]
    pub fn color(self) -> Option<Hsla> {
        let hex = match self {
            Self::Plain => return None,
            Self::Attention => AMBER_500,
            Self::Closed => RED_500,
            Self::Open => GREEN_500,
            Self::Merged => VIOLET_500,
        };
        Some(rgb(hex).into())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(GlyphTone, bool); 5] = [
        (GlyphTone::Plain, false),
        (GlyphTone::Attention, true),
        (GlyphTone::Closed, true),
        (GlyphTone::Open, true),
        (GlyphTone::Merged, true),
    ];

    #[test]
    fn only_the_plain_tone_follows_the_text() {
        for (tone, fixed) in CASES {
            assert_eq!(tone.color().is_some(), fixed, "{tone:?}");
        }
    }
}
