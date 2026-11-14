use super::*;
use crate::theme::Theme;

fn plain() -> MenuAction {
    MenuAction::new("x", |_, _| {})
}

fn dangerous() -> MenuAction {
    plain().destructive()
}

#[test]
fn plain_rows_highlight_with_the_accent_colours() {
    let palette = Theme::default().palette(Surface::Window);
    let plain = plain();
    assert_eq!(
        colors(&plain, Pose::Off, palette, Appearance::Light).1,
        palette.foreground()
    );
    assert_eq!(
        colors(&plain, Pose::On, palette, Appearance::Light),
        (palette.accent(), palette.accent_foreground())
    );
}

#[test]
fn destructive_rows_tint_more_strongly_in_dark_mode() {
    let palette = Theme::default().palette(Surface::Window);
    let danger = dangerous();
    assert_eq!(
        colors(&danger, Pose::Off, palette, Appearance::Light).1,
        palette.destructive()
    );
    let light = colors(&danger, Pose::On, palette, Appearance::Light).0;
    let dark = colors(&danger, Pose::On, palette, Appearance::Dark).0;
    assert!(dark.a > light.a);
}
