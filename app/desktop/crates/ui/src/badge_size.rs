//! Sizes of a badge.

use gpui::{Pixels, px};

use crate::text_size;

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
        px(match self {
            Self::Sm => 16.0,
            Self::Md => 18.0,
            Self::Lg => 22.0,
        })
    }

    /// Horizontal padding inside the 1px border.
    pub fn padding_x(self) -> Pixels {
        px(match self {
            Self::Sm | Self::Md => 3.0,
            Self::Lg => 5.0,
        })
    }

    /// Font size.
    pub fn text(self) -> Pixels {
        match self {
            Self::Sm => px(10.0),
            Self::Md => text_size::XS,
            Self::Lg => text_size::SM,
        }
    }

    /// Corner radius.
    pub fn radius(self) -> Pixels {
        match self {
            Self::Sm => px(4.0),
            _ => crate::radius::SM,
        }
    }
}

#[cfg(test)]
#[path = "badge_size_tests.rs"]
mod tests;
