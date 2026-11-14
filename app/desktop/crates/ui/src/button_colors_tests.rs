use super::*;
use crate::contrast::contrast_ratio;
use crate::surface::Surface;
use crate::theme::Theme;
use crate::theme_mode::ThemeMode;

fn palette(mode: ThemeMode) -> (&'static Palette, Appearance) {
    let theme = Theme::default().with_mode(mode);
    (theme.palette(Surface::Window), theme.appearance())
}

#[test]
fn filled_text_is_readable_in_both_modes() {
    for mode in [ThemeMode::Light, ThemeMode::Dark] {
        let (palette, appearance) = palette(mode);
        for variant in [ButtonVariant::Primary, ButtonVariant::Destructive, ButtonVariant::Secondary] {
            let colors = ButtonColors::resolve(variant, palette, appearance);
            let ratio = contrast_ratio(colors.foreground, colors.background);
            assert!(ratio >= 4.5, "{variant:?} {mode:?}: {ratio:.2}");
        }
    }
}

#[test]
fn hover_of_filled_variants_is_ninety_percent() {
    let (palette, appearance) = palette(ThemeMode::Light);
    let colors = ButtonColors::resolve(ButtonVariant::Primary, palette, appearance);
    assert!((colors.hover_background.a - 0.9).abs() < 1e-6);
    assert_eq!(colors.shadow.len(), 1);
}

#[test]
fn ghost_is_transparent_until_hover() {
    let (palette, appearance) = palette(ThemeMode::Dark);
    let colors = ButtonColors::resolve(ButtonVariant::Ghost, palette, appearance);
    assert_eq!(colors.background.a, 0.0);
    assert_eq!(colors.hover_background, palette.accent());
}
