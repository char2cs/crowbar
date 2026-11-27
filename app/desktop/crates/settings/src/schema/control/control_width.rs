const NUMBER_COMPACT: f32 = 96.0;
const COMPACT: f32 = 112.0;
const DEFAULT: f32 = 144.0;
const WIDE: f32 = 176.0;
const TEXT: f32 = 192.0;
const EXTRA_WIDE: f32 = 224.0;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum ControlWidth {
    Compact,
    Default,
    Wide,
    ExtraWide,
    NumberCompact,
    Text,
    TextWide,
}

impl ControlWidth {
    pub fn pixels(self) -> f32 {
        match self {
            Self::NumberCompact => NUMBER_COMPACT,
            Self::Compact => COMPACT,
            Self::Default => DEFAULT,
            Self::Wide => WIDE,
            Self::Text => TEXT,
            Self::ExtraWide | Self::TextWide => EXTRA_WIDE,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn widths_match_the_spec_table() {
        const CASES: &[(ControlWidth, f32)] = &[
            (ControlWidth::Compact, 112.0),
            (ControlWidth::Default, 144.0),
            (ControlWidth::Wide, 176.0),
            (ControlWidth::ExtraWide, 224.0),
            (ControlWidth::NumberCompact, 96.0),
            (ControlWidth::Text, 192.0),
            (ControlWidth::TextWide, 224.0),
        ];
        for &(width, pixels) in CASES {
            assert_eq!(width.pixels(), pixels, "{width:?}");
        }
    }
}
