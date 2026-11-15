//! The `ui-text-*` sizes that follow the UI font setting.

use gpui::{Pixels, px};

use crate::text_size;
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

const SIZE_XS: f32 = 11.0;

impl UiText {
    /// Size in pixels under `theme`.
    pub fn size(
        self,
        theme: &Theme,
    ) -> Pixels {
        let base = match self {
            Self::Xs => SIZE_XS,
            Self::Sm => f32::from(text_size::XS),
            Self::Base => f32::from(text_size::SM),
            Self::Lg => f32::from(text_size::BASE),
            Self::Xl => f32::from(text_size::XL),
        };
        px(base * theme.ui_scale())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ui_text_scales_with_the_setting() {
        const CASES: &[(UiText, f32, f32)] = &[
            (UiText::Xs, 15.0, 11.0),
            (UiText::Sm, 15.0, 12.0),
            (UiText::Base, 15.0, 14.0),
            (UiText::Sm, 30.0, 19.2),
            (UiText::Xl, 12.0, 16.0),
        ];
        for &(size, setting, expected) in CASES {
            let theme = Theme::default().with_ui_font_size(setting);
            let actual: f32 = size.size(&theme).into();
            assert!(
                (actual - expected).abs() < 1e-4,
                "{size:?} at {setting}: {actual}"
            );
        }
    }
}
