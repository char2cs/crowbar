//! The `ui-text-*` sizes that follow the UI font setting.

use gpui::{Pixels, px};

use crate::theme::Theme;

/// The `ui-text-*` sizes, multiplied by `--app-ui-scale`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum UiText {
    /// `ui-text-xs`, 11px at scale 1.
    Xs,
    /// `ui-text-sm`, 12px at scale 1.
    Sm,
    /// `ui-text-base`, 14px at scale 1.
    Base,
    /// `ui-text-lg`, 16px at scale 1.
    Lg,
    /// `ui-text-xl`, 20px at scale 1.
    Xl,
}

impl UiText {
    /// Size in pixels under `theme`.
    pub fn size(self, theme: &Theme) -> Pixels {
        let base = match self {
            Self::Xs => 11.0,
            Self::Sm => 12.0,
            Self::Base => 14.0,
            Self::Lg => 16.0,
            Self::Xl => 20.0,
        };
        px(base * theme.ui_scale())
    }
}

#[cfg(test)]
#[path = "ui_text_tests.rs"]
mod tests;
