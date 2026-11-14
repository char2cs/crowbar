use super::*;
use crate::surface::Surface;
use crate::theme::Theme;

#[test]
fn focus_adds_the_ring_and_recolours_the_border() {
    let palette = Theme::default().palette(Surface::Window);
    let idle = chrome(palette, Appearance::Light, Pose::Off);
    let focused = chrome(palette, Appearance::Light, Pose::On);
    assert_eq!(idle.border, palette.input());
    assert_eq!(focused.border, palette.ring());
    assert_eq!(focused.shadows.len(), idle.shadows.len() + 1);
}

#[test]
fn dark_mode_fills_with_a_translucent_input_colour() {
    let palette = Theme::default().palette(Surface::Window);
    assert_eq!(
        chrome(palette, Appearance::Light, Pose::Off).fill,
        palette.background()
    );
    assert!(chrome(palette, Appearance::Dark, Pose::Off).fill.a < 1.0);
}
