//! Size scale of buttons.

use gpui::{Pixels, px};

use crate::foundation::ControlScale;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::spacing;
use crate::foundation::tokens::text_size;

const ICON_XL: Pixels = px(18.0);
const BASE_ICON_GROWTH: f32 = 2.0;

/// Height steps of a button; the icon-only form is square at the same size.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum ButtonSize {
    /// 24px.
    Xs,
    /// 28px.
    Sm,
    /// 32px, the default.
    #[default]
    Md,
    /// 36px.
    Lg,
    /// 40px.
    Xl,
}

impl ButtonSize {
    /// Outer height (and width of the icon-only form).
    pub fn height(self) -> Pixels {
        match self {
            Self::Xs => spacing::STEP_6,
            Self::Sm => spacing::STEP_7,
            Self::Md => spacing::STEP_8,
            Self::Lg => spacing::STEP_9,
            Self::Xl => spacing::STEP_10,
        }
    }

    /// Horizontal padding inside the 1px border.
    pub fn padding_x(self) -> Pixels {
        match self {
            Self::Xs => spacing::STEP_1_5,
            Self::Sm => spacing::STEP_2,
            Self::Md => spacing::STEP_2_5,
            Self::Lg => spacing::STEP_3,
            Self::Xl => spacing::STEP_3_5,
        }
    }

    /// Gap between icon and label.
    pub fn gap(self) -> Pixels {
        match self {
            Self::Xs => spacing::STEP_1,
            Self::Sm => spacing::STEP_1_5,
            _ => spacing::STEP_2,
        }
    }

    /// Label font size.
    pub fn text(self) -> Pixels {
        match self {
            Self::Xs => text_size::XS,
            Self::Xl => text_size::BASE,
            _ => text_size::SM,
        }
    }

    /// Icon edge length.
    pub fn icon(self) -> Pixels {
        match self {
            Self::Xs => spacing::STEP_3_5,
            Self::Xl => ICON_XL,
            _ => spacing::STEP_4,
        }
    }

    pub(crate) fn sized(
        self,
        scale: ControlScale,
    ) -> (Pixels, Pixels, Pixels) {
        if scale == ControlScale::Sm {
            return (self.height(), self.text(), self.icon());
        }
        let text = match self {
            Self::Xs => text_size::SM,
            Self::Xl => text_size::LG,
            _ => text_size::BASE,
        };
        (
            self.height() + scale.growth(),
            text,
            self.icon() + px(BASE_ICON_GROWTH),
        )
    }

    /// Corner radius.
    pub fn radius(self) -> Pixels {
        match self {
            Self::Xs => radius::MD,
            _ => radius::LG,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn heights_match_the_spec() {
        let cases = [
            (ButtonSize::Xs, 24.0, 6.0, 14.0),
            (ButtonSize::Sm, 28.0, 8.0, 16.0),
            (ButtonSize::Md, 32.0, 10.0, 16.0),
            (ButtonSize::Lg, 36.0, 12.0, 16.0),
            (ButtonSize::Xl, 40.0, 14.0, 18.0),
        ];
        for (size, height, padding, icon) in cases {
            assert_eq!(f32::from(size.height()), height, "{size:?} height");
            assert_eq!(f32::from(size.padding_x()), padding, "{size:?} padding");
            assert_eq!(f32::from(size.icon()), icon, "{size:?} icon");
            let sm = size.sized(ControlScale::Sm);
            assert_eq!(sm, (size.height(), size.text(), size.icon()), "{size:?} sm");
            let (base_height, _, base_icon) = size.sized(ControlScale::Base);
            assert_eq!(f32::from(base_height), height + 4.0, "{size:?} base height");
            assert_eq!(f32::from(base_icon), icon + 2.0, "{size:?} base icon");
        }
    }
}
