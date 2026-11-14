use super::*;
use crate::surface::Surface;
use crate::theme::Theme;

#[test]
fn tinted_variants_use_a_faint_fill_that_strengthens_in_dark_mode() {
    let theme = Theme::default();
    let palette = theme.palette(Surface::Window);
    let light = BadgePaint::of(BadgeVariant::Success, palette, Appearance::Light);
    let dark = BadgePaint::of(BadgeVariant::Success, palette, Appearance::Dark);
    assert!((light.fill.a - TINT_LIGHT).abs() < 1e-6);
    assert!((dark.fill.a - TINT_DARK).abs() < 1e-6);
    assert_eq!(dark.text, palette.success());
}

#[test]
fn solid_variants_are_opaque_without_border() {
    let theme = Theme::default();
    let palette = theme.palette(Surface::Window);
    let primary = BadgePaint::of(BadgeVariant::Primary, palette, Appearance::Light);
    assert_eq!(primary.fill, palette.primary());
    assert!(primary.border.a.abs() < f32::EPSILON);
}

#[test]
fn outline_has_a_border() {
    let theme = Theme::default();
    let palette = theme.palette(Surface::Window);
    let outline = BadgePaint::of(BadgeVariant::Outline, palette, Appearance::Light);
    assert_eq!(outline.border, palette.input());
}
