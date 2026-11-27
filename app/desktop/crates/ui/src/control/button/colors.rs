use gpui::{App, BoxShadow, Entity, Hsla, StyleRefinement, Styled, Window, hsla};

use crate::control::button::button_variant::ButtonVariant;
use crate::foundation::Appearance;
use crate::foundation::Palette;
use crate::foundation::motion::ToggleState;
use crate::foundation::motion::hover;
use crate::foundation::tokens::opacity;
use crate::foundation::tokens::shadow;

const HOVER_FILL: f32 = 0.9;
const GLOW: f32 = 0.24;
const PRESSED_FILL: f32 = 0.8;
const FAINT_INK: f32 = 0.6;

#[derive(Debug, Clone)]
pub(crate) struct Colors {
    pub(crate) background: Hsla,
    pub(crate) hover_background: Hsla,
    pub(crate) pressed_background: Hsla,
    pub(crate) border: Hsla,
    pub(crate) foreground: Hsla,
    pub(crate) hover_foreground: Hsla,
    pub(crate) shadow: Vec<BoxShadow>,
}

fn transparent() -> Hsla {
    hsla(0.0, 0.0, 0.0, 0.0)
}

fn white() -> Hsla {
    hsla(0.0, 0.0, 1.0, 1.0)
}

impl Colors {
    pub(crate) fn fill(
        &self,
        hover: &Entity<ToggleState>,
        (window, cx): (&mut Window, &App),
    ) -> Hsla {
        hover::color(hover, self.background..self.hover_background, window, cx)
    }

    pub(crate) fn inked(
        mut self,
        hover: &Entity<ToggleState>,
        (window, cx): (&mut Window, &App),
    ) -> Self {
        let ink = self.foreground..self.hover_foreground;
        self.foreground = hover::color(hover, ink, window, cx);
        self
    }

    pub(crate) fn pressed(&self) -> impl FnOnce(StyleRefinement) -> StyleRefinement + use<> {
        let (fill, shadow) = (self.pressed_background, Vec::new());
        move |style| style.bg(fill).shadow(shadow)
    }

    pub(crate) fn resolve(
        variant: ButtonVariant,
        palette: &Palette,
        appearance: Appearance,
    ) -> Self {
        match variant {
            ButtonVariant::Primary => Self::filled(palette.primary(), palette.primary_foreground()),
            ButtonVariant::Destructive => {
                Self::filled(palette.destructive(), destructive_text(palette, appearance))
            },
            ButtonVariant::Secondary => Self {
                pressed_background: palette.secondary().opacity(PRESSED_FILL),
                ..Self::plain(palette.secondary(), palette.secondary_foreground())
            },
            ButtonVariant::Outline => Self::outline(palette, appearance),
            ButtonVariant::Ghost => Self {
                background: transparent(),
                hover_background: palette.accent(),
                pressed_background: palette.accent(),
                border: transparent(),
                foreground: palette.foreground(),
                hover_foreground: palette.foreground(),
                shadow: Vec::new(),
            },
            ButtonVariant::Muted | ButtonVariant::Row => Self {
                foreground: palette.muted_foreground(),
                hover_foreground: palette.muted_foreground(),
                ..Self::resolve(ButtonVariant::Ghost, palette, appearance)
            },
            ButtonVariant::Faint => Self {
                foreground: palette.muted_foreground().opacity(FAINT_INK),
                hover_foreground: palette.destructive(),
                ..Self::resolve(ButtonVariant::Ghost, palette, appearance)
            },
        }
    }

    fn filled(
        fill: Hsla,
        foreground: Hsla,
    ) -> Self {
        Self {
            shadow: shadow::xs_tinted(fill.opacity(GLOW)),
            ..Self::plain(fill, foreground)
        }
    }

    fn plain(
        fill: Hsla,
        foreground: Hsla,
    ) -> Self {
        Self {
            background: fill,
            hover_background: fill.opacity(HOVER_FILL),
            pressed_background: fill.opacity(HOVER_FILL),
            border: fill,
            foreground,
            hover_foreground: foreground,
            shadow: Vec::new(),
        }
    }

    fn outline(
        palette: &Palette,
        appearance: Appearance,
    ) -> Self {
        let (background, hover_background) = match appearance {
            Appearance::Light => (palette.popover(), palette.accent().opacity(0.5)),
            Appearance::Dark => (
                palette.input().opacity(opacity::DARK_INPUT),
                palette.input().opacity(opacity::DARK_INPUT_HOVER),
            ),
        };
        Self {
            background,
            hover_background,
            pressed_background: hover_background,
            border: palette.input(),
            foreground: palette.foreground(),
            hover_foreground: palette.foreground(),
            shadow: shadow::xs(opacity::PERCENT_5),
        }
    }
}

fn destructive_text(
    palette: &Palette,
    appearance: Appearance,
) -> Hsla {
    match appearance {
        Appearance::Light => white(),
        Appearance::Dark => palette.destructive_foreground(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::foundation::Surface;
    use crate::foundation::Theme;
    use crate::foundation::ThemeMode;
    use crate::test_support::contrast_ratio;

    fn palette(mode: ThemeMode) -> (&'static Palette, Appearance) {
        let theme = Theme::default().with_mode(mode);
        (theme.palette(Surface::Window), theme.appearance())
    }

    #[test]
    fn filled_text_is_readable_in_both_modes() {
        const CASES: [ButtonVariant; 3] = [
            ButtonVariant::Primary,
            ButtonVariant::Destructive,
            ButtonVariant::Secondary,
        ];
        let pairs = [ThemeMode::Light, ThemeMode::Dark]
            .into_iter()
            .flat_map(|mode| CASES.map(|variant| (mode, variant)));
        for (mode, variant) in pairs {
            let (palette, appearance) = palette(mode);
            let colors = Colors::resolve(variant, palette, appearance);
            let ratio = contrast_ratio(colors.foreground, colors.background);
            assert!(ratio >= 4.5, "{variant:?} {mode:?}: {ratio:.2}");
        }
    }

    #[test]
    fn hover_of_filled_variants_is_ninety_percent() {
        let (palette, appearance) = palette(ThemeMode::Light);
        let colors = Colors::resolve(ButtonVariant::Primary, palette, appearance);
        assert!((colors.hover_background.a - 0.9).abs() < 1e-6);
        assert_eq!(colors.shadow.len(), 1);
    }

    #[test]
    fn only_secondary_presses_darker_than_it_hovers() {
        const CASES: &[(ButtonVariant, f32)] = &[
            (ButtonVariant::Primary, 0.9),
            (ButtonVariant::Destructive, 0.9),
            (ButtonVariant::Secondary, 0.8),
        ];
        let (palette, appearance) = palette(ThemeMode::Light);
        for &(variant, alpha) in CASES {
            let colors = Colors::resolve(variant, palette, appearance);
            assert!(
                (colors.pressed_background.a - alpha).abs() < 1e-6,
                "{variant:?}"
            );
        }
        let outline = Colors::resolve(ButtonVariant::Outline, palette, appearance);
        assert_eq!(outline.pressed_background, outline.hover_background);
    }

    #[test]
    fn ghost_and_row_are_transparent_until_hover_and_a_row_inks_muted() {
        let (palette, appearance) = palette(ThemeMode::Dark);
        let cases = [
            (ButtonVariant::Ghost, palette.foreground()),
            (ButtonVariant::Muted, palette.muted_foreground()),
            (ButtonVariant::Row, palette.muted_foreground()),
        ];
        for (variant, ink) in cases {
            let colors = Colors::resolve(variant, palette, appearance);
            assert_eq!(colors.background.a, 0.0, "{variant:?}");
            assert_eq!(colors.hover_background, palette.accent(), "{variant:?}");
            assert_eq!(colors.foreground, ink, "{variant:?}");
        }
    }

    #[test]
    fn a_faint_ghost_inks_muted_at_sixty_percent_and_turns_destructive_under_the_pointer() {
        let (palette, appearance) = palette(ThemeMode::Dark);
        let faint = Colors::resolve(ButtonVariant::Faint, palette, appearance);
        let inks = (faint.foreground, faint.hover_foreground);
        let want = (
            palette.muted_foreground().opacity(0.6),
            palette.destructive(),
        );
        assert_eq!(inks, want);
    }
}
