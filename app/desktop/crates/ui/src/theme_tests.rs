use gpui::Hsla;

use super::*;

fn theme(id: ThemeId, mode: ThemeMode, system: Appearance) -> Theme {
    Theme::default().with_id(id).with_mode(mode).with_system(system)
}

#[test]
fn system_mode_follows_the_os() {
    let dark = theme(ThemeId::Crowbar, ThemeMode::System, Appearance::Dark);
    let light = dark.with_system(Appearance::Light);
    assert_eq!(dark.appearance(), Appearance::Dark);
    assert_eq!(light.appearance(), Appearance::Light);
    assert_ne!(dark.palette(Surface::Window), light.palette(Surface::Window));
}

#[test]
fn crowbar_chrome_stays_dark_in_light_mode() {
    let light = theme(ThemeId::Crowbar, ThemeMode::Light, Appearance::Light);
    let dark = theme(ThemeId::Crowbar, ThemeMode::Dark, Appearance::Light);
    assert_eq!(
        light.palette(Surface::Chrome).background(),
        dark.palette(Surface::Window).background(),
    );
    assert_ne!(
        light.palette(Surface::Chrome).background(),
        light.palette(Surface::Window).background(),
    );
}

#[test]
fn frosted_has_no_special_chrome() {
    let light = theme(ThemeId::Frosted, ThemeMode::Light, Appearance::Light);
    assert_eq!(light.palette(Surface::Chrome), light.palette(Surface::Window));
}

#[test]
fn ui_scale_is_size_over_fifteen_clamped() {
    const CASES: &[(f32, f32)] = &[(15.0, 1.0), (30.0, 1.6), (5.0, 10.0 / 15.0), (12.0, 0.8)];
    for &(size, scale) in CASES {
        let actual = Theme::default().with_ui_font_size(size).ui_scale();
        assert!((actual - scale).abs() < 1e-6, "size {size}: {actual}");
    }
}

#[test]
fn spec_literals_survive() {
    let palette = theme(ThemeId::Crowbar, ThemeMode::Dark, Appearance::Dark).palette(Surface::Window);
    assert_eq!(palette.primary(), Hsla::from(crate::color::hex(0x98a86aff)));
    assert_eq!(palette.border(), Hsla::from(crate::color::hex(0x333330ff)));
}
