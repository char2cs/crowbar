//! Width steps of an application dialog.

use gpui::{Pixels, px};

/// Maximum width of a [`crate::Dialog`].
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum DialogSize {
    /// 384px.
    Sm,
    /// 448px.
    #[default]
    Md,
    /// 672px.
    Lg,
    /// 896px.
    Xl,
}

impl DialogSize {
    /// The maximum popup width.
    pub fn max_width(self) -> Pixels {
        px(match self {
            Self::Sm => 384.0,
            Self::Md => 448.0,
            Self::Lg => 672.0,
            Self::Xl => 896.0,
        })
    }
}

#[cfg(test)]
#[path = "dialog_size_tests.rs"]
mod tests;
