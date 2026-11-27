use gpui::{Pixels, px};

use crate::foundation::tokens::radius;
use crate::foundation::tokens::text_size;

const BORDER: f32 = 1.0;
const DEFAULT_MIN: f32 = 70.0;
const SMALL_MIN: f32 = 66.0;
const LARGE_MIN: f32 = 74.0;
const DEFAULT_ACROSS: f32 = 11.0;
const SMALL_ACROSS: f32 = 9.0;
const DEFAULT_DOWN: f32 = 5.0;
const SMALL_DOWN: f32 = 3.0;
const LARGE_DOWN: f32 = 7.0;
const FIELD_MIN: f32 = 48.0;
const FIELD_ACROSS: f32 = 8.0;
const FIELD_DOWN: f32 = 6.0;
const FIELD_LINE: f32 = 18.0;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum TextAreaSize {
    Small,
    #[default]
    Default,
    Large,
    Field,
}

impl TextAreaSize {
    pub fn min_height(self) -> Pixels {
        px(BORDER * 2.0
            + match self {
                Self::Small => SMALL_MIN,
                Self::Default => DEFAULT_MIN,
                Self::Large => LARGE_MIN,
                Self::Field => FIELD_MIN,
            })
    }

    pub(crate) fn across(self) -> Pixels {
        px(match self {
            Self::Small => SMALL_ACROSS,
            Self::Default | Self::Large => DEFAULT_ACROSS,
            Self::Field => FIELD_ACROSS,
        })
    }

    pub(crate) fn down(self) -> Pixels {
        px(match self {
            Self::Small => SMALL_DOWN,
            Self::Default => DEFAULT_DOWN,
            Self::Large => LARGE_DOWN,
            Self::Field => FIELD_DOWN,
        })
    }

    pub(crate) fn line_height(self) -> Pixels {
        match self {
            Self::Field => px(FIELD_LINE),
            Self::Small | Self::Default | Self::Large => text_size::LINE_HEIGHT,
        }
    }

    pub(crate) fn text(self) -> Pixels {
        match self {
            Self::Field => text_size::XS,
            Self::Small | Self::Default | Self::Large => text_size::SM,
        }
    }

    pub(crate) fn radius(self) -> Pixels {
        match self {
            Self::Field => radius::MD,
            Self::Small | Self::Default | Self::Large => radius::LG,
        }
    }

    pub(crate) fn frame_height(self) -> Pixels {
        self.down() * 2.0 + px(BORDER * 2.0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn metrics_follow_the_react_textarea() {
        type Case = (TextAreaSize, f32, f32, f32);
        const CASES: &[Case] = &[
            (TextAreaSize::Small, 68.0, 9.0, 3.0),
            (TextAreaSize::Default, 72.0, 11.0, 5.0),
            (TextAreaSize::Large, 76.0, 11.0, 7.0),
            (TextAreaSize::Field, 50.0, 8.0, 6.0),
        ];
        for &(size, min, across, down) in CASES {
            assert_eq!(size.min_height(), px(min), "{size:?} min");
            assert_eq!(size.across(), px(across), "{size:?} across");
            assert_eq!(size.down(), px(down), "{size:?} down");
        }
    }

    #[test]
    fn the_frame_is_padding_and_border_above_and_below() {
        assert_eq!(TextAreaSize::Default.frame_height(), px(12.0));
    }
}
