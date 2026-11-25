//! Size and finish of a [`crate::TextInput`].

use gpui::{Pixels, px};

use crate::foundation::ControlScale;
use crate::foundation::ScaledText;
use crate::foundation::Theme;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::text_size;

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
    /// [`Self::Bare`] in the mono family, for a branch name.
    BareMono,
    /// [`Self::Standard`] framed in the foreground colour without a focus ring, as develop's
    /// Files-tree name field (`border-foreground`, focused by script so never focus-visible).
    Outlined,
}

impl TextInputStyle {
    /// Outer height including the border.
    pub fn height(self) -> Pixels {
        px(match self {
            Self::Standard | Self::Outlined => STANDARD_HEIGHT,
            Self::Compact => COMPACT_HEIGHT,
            Self::Stepper => STEPPER_HEIGHT,
            Self::Bare | Self::BareMono => BARE_HEIGHT,
        })
    }

    pub(crate) fn padding(self) -> Pixels {
        px(match self {
            Self::Standard | Self::Outlined => STANDARD_PADDING,
            Self::Compact => COMPACT_PADDING,
            Self::Stepper => STEPPER_PADDING,
            Self::Bare | Self::BareMono => 0.0,
        })
    }

    pub(crate) fn radius(self) -> Pixels {
        match self {
            Self::Stepper => radius::MD,
            Self::Bare | Self::BareMono => px(0.0),
            Self::Standard | Self::Compact | Self::Outlined => radius::LG,
        }
    }

    pub(crate) fn sized(
        self,
        theme: Theme,
        point: ControlScale,
    ) -> (Pixels, Pixels) {
        match self {
            Self::Standard | Self::Compact | Self::Outlined => {
                (self.height() + point.growth(), point.text())
            },
            Self::Stepper | Self::Bare | Self::BareMono => (self.height(), self.text(theme)),
        }
    }

    fn text(
        self,
        theme: Theme,
    ) -> Pixels {
        match self {
            Self::Bare | Self::BareMono => px(BARE_TEXT),
            Self::Stepper => ScaledText::Sm.size(&theme),
            Self::Standard | Self::Compact | Self::Outlined => text_size::SM,
        }
    }

    pub(crate) fn family(
        self,
        theme: Theme,
    ) -> &'static str {
        if self == Self::BareMono {
            return theme.mono_family();
        }
        theme.ui_family()
    }

    pub(crate) fn bare(self) -> bool {
        matches!(self, Self::Bare | Self::BareMono)
    }

    pub(crate) fn centered(self) -> bool {
        self == Self::Stepper
    }
}

#[cfg(test)]
mod tests {
    use gpui::{App, AppContext as _, ParentElement as _, Styled as _, TestAppContext, px};

    use super::*;
    use crate::test_support::probe::measure;
    use crate::{IconName, TextInput};

    fn field_of(
        style: TextInputStyle,
        cx: &mut App,
    ) -> gpui::Div {
        let input = cx.new(|cx| {
            TextInput::new("x", cx)
                .style(style)
                .leading_icon(IconName::MagnifyingGlass)
        });
        gpui::div().w(px(120.0)).child(input)
    }

    #[test]
    fn heights_follow_the_style() {
        const CASES: &[(TextInputStyle, f32)] = &[
            (TextInputStyle::Standard, 32.0),
            (TextInputStyle::Outlined, 32.0),
            (TextInputStyle::Compact, 28.0),
            (TextInputStyle::Stepper, 24.0),
            (TextInputStyle::Bare, 20.0),
        ];
        for &(style, height) in CASES {
            assert_eq!(style.height(), px(height), "{style:?}");
        }
    }

    #[test]
    fn the_bare_mono_field_draws_in_the_mono_family_and_the_others_in_the_ui_family() {
        let theme = Theme::default();
        assert_eq!(TextInputStyle::BareMono.family(theme), theme.mono_family());
        assert_eq!(TextInputStyle::Bare.family(theme), theme.ui_family());
        assert_eq!(
            TextInputStyle::BareMono.height(),
            TextInputStyle::Bare.height()
        );
    }

    #[test]
    fn only_the_stepper_is_centred() {
        assert!(TextInputStyle::Stepper.centered());
        assert!(!TextInputStyle::Compact.centered());
    }

    #[gpui::test]
    fn the_rendered_field_has_the_style_height(cx: &mut TestAppContext) {
        const CASES: &[(TextInputStyle, f32)] = &[
            (TextInputStyle::Compact, 28.0),
            (TextInputStyle::Stepper, 24.0),
        ];
        for &(style, height) in CASES {
            let bounds = measure(cx, move |_, cx| field_of(style, cx));
            assert_eq!(f32::from(bounds.size.height), height, "{style:?}");
        }
    }
}
