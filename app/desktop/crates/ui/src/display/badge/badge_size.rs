use gpui::{Pixels, px};

use crate::foundation::tokens::spacing;
use crate::foundation::tokens::text_size;

const EXTENT_MD: Pixels = px(18.0);
const EXTENT_LG: Pixels = px(22.0);
const PADDING_X_COMPACT: Pixels = px(3.0);
const PADDING_X_LARGE: Pixels = px(5.0);
const TEXT_SM: Pixels = px(10.0);

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum BadgeSize {
    Sm,
    #[default]
    Md,
    Lg,
}

impl BadgeSize {
    pub fn extent(self) -> Pixels {
        match self {
            Self::Sm => spacing::STEP_4,
            Self::Md => EXTENT_MD,
            Self::Lg => EXTENT_LG,
        }
    }

    pub fn padding_x(self) -> Pixels {
        match self {
            Self::Sm | Self::Md => PADDING_X_COMPACT,
            Self::Lg => PADDING_X_LARGE,
        }
    }

    pub fn text(self) -> Pixels {
        match self {
            Self::Sm => TEXT_SM,
            Self::Md => text_size::XS,
            Self::Lg => text_size::SM,
        }
    }

    pub fn radius(self) -> Pixels {
        match self {
            Self::Sm => spacing::STEP_1,
            _ => crate::foundation::tokens::radius::SM,
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
