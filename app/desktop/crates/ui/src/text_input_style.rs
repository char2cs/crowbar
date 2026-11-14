//! Size and finish of a [`crate::TextInput`].

use gpui::{Pixels, px};

use crate::radius;
use crate::text_size;
use crate::theme::Theme;
use crate::ui_text::UiText;

const STANDARD_HEIGHT: f32 = 32.0;
const COMPACT_HEIGHT: f32 = 28.0;
const STEPPER_HEIGHT: f32 = 24.0;
const BARE_HEIGHT: f32 = 20.0;
const BARE_TEXT: f32 = 13.0;
const STANDARD_PADDING: f32 = 10.0;
const COMPACT_PADDING: f32 = 9.0;
const STEPPER_PADDING: f32 = 8.0;

/// How tall and how finished a text field is.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum TextInputStyle {
    /// 32px field with the control radius.
    #[default]
    Standard,
    /// 28px field for rows of controls (the `sm` input).
    Compact,
    /// 24px centred field on the muted fill, used between a stepper's buttons.
    Stepper,
    /// Frameless 13px medium field that replaces a row label while it is renamed.
    Bare,
}

impl TextInputStyle {
    /// Outer height including the border.
    pub fn height(self) -> Pixels {
        px(match self {
            Self::Standard => STANDARD_HEIGHT,
            Self::Compact => COMPACT_HEIGHT,
            Self::Stepper => STEPPER_HEIGHT,
            Self::Bare => BARE_HEIGHT,
        })
    }

    pub(crate) fn padding(self) -> Pixels {
        px(match self {
            Self::Standard => STANDARD_PADDING,
            Self::Compact => COMPACT_PADDING,
            Self::Stepper => STEPPER_PADDING,
            Self::Bare => 0.0,
        })
    }

    pub(crate) fn radius(self) -> Pixels {
        match self {
            Self::Stepper => radius::MD,
            Self::Bare => px(0.0),
            Self::Standard | Self::Compact => radius::LG,
        }
    }

    pub(crate) fn text(self, theme: Theme) -> Pixels {
        match self {
            Self::Bare => px(BARE_TEXT),
            Self::Stepper => UiText::Sm.size(&theme),
            Self::Standard | Self::Compact => text_size::SM,
        }
    }

    pub(crate) fn centered(self) -> bool {
        self == Self::Stepper
    }
}

#[cfg(test)]
#[path = "text_input_style_tests.rs"]
mod tests;
