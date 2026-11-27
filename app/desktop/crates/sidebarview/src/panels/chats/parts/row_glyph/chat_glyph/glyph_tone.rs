use gpui::{Hsla, rgb};

const AMBER_500: u32 = 0x00fe_9a00;
const RED_500: u32 = 0x00fb_2c36;
const GREEN_500: u32 = 0x0000_c950;
const VIOLET_500: u32 = 0x008e_51ff;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum GlyphTone {
    Plain,
    Attention,
    Closed,
    Open,
    Merged,
}

impl GlyphTone {
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
