//! Sizes of a badge.

use gpui::{Pixels, px};

use crate::spacing;
use crate::text_size;

const EXTENT_MD: Pixels = px(18.0);
const EXTENT_LG: Pixels = px(22.0);
const PADDING_X_COMPACT: Pixels = px(3.0);
const PADDING_X_LARGE: Pixels = px(5.0);
const TEXT_SM: Pixels = px(10.0);

/// Height steps of a [`crate::Badge`].
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum BadgeSize {
    /// 16px high, 10px text.
    Sm,
    /// 18px high, 12px text.
    #[default]
    Md,
    /// 22px high, 14px text.
    Lg,
}

impl BadgeSize {
    /// Height and minimum width.
    pub fn extent(self) -> Pixels {
        match self {
            Self::Sm => spacing::STEP_4,
            Self::Md => EXTENT_MD,
            Self::Lg => EXTENT_LG,
        }
    }

    /// Horizontal padding inside the 1px border.
    pub fn padding_x(self) -> Pixels {
        match self {
            Self::Sm | Self::Md => PADDING_X_COMPACT,
            Self::Lg => PADDING_X_LARGE,
        }
    }

    /// Font size.
    pub fn text(self) -> Pixels {
        match self {
            Self::Sm => TEXT_SM,
            Self::Md => text_size::XS,
            Self::Lg => text_size::SM,
        }
    }

    /// Corner radius.
    pub fn radius(self) -> Pixels {
        match self {
            Self::Sm => spacing::STEP_1,
            _ => crate::radius::SM,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sizes_match_the_spec() {
        let cases = [
            (BadgeSize::Sm, 16.0, 10.0, 4.0),
            (BadgeSize::Md, 18.0, 12.0, 6.0),
            (BadgeSize::Lg, 22.0, 14.0, 6.0),
        ];
        for (size, extent, text, radius) in cases {
            assert_eq!(f32::from(size.extent()), extent, "{size:?}");
            assert_eq!(f32::from(size.text()), text, "{size:?}");
            assert_eq!(f32::from(size.radius()), radius, "{size:?}");
        }
    }
}
