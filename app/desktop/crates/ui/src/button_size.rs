//! Size scale of buttons.

use gpui::{Pixels, px};

use crate::radius;
use crate::text_size;

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
        px(match self {
            Self::Xs => 24.0,
            Self::Sm => 28.0,
            Self::Md => 32.0,
            Self::Lg => 36.0,
            Self::Xl => 40.0,
        })
    }

    /// Horizontal padding inside the 1px border.
    pub fn padding_x(self) -> Pixels {
        px(match self {
            Self::Xs => 6.0,
            Self::Sm => 8.0,
            Self::Md => 10.0,
            Self::Lg => 12.0,
            Self::Xl => 14.0,
        })
    }

    /// Gap between icon and label.
    pub fn gap(self) -> Pixels {
        px(match self {
            Self::Xs => 4.0,
            Self::Sm => 6.0,
            _ => 8.0,
        })
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
        px(match self {
            Self::Xs => 14.0,
            Self::Xl => 18.0,
            _ => 16.0,
        })
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
#[path = "button_size_tests.rs"]
mod tests;
