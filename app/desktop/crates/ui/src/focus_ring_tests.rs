use super::*;
use crate::theme::Theme;
use crate::surface::Surface;

#[test]
fn ring_is_two_pixels_beyond_a_one_pixel_offset() {
    let palette = Theme::default().palette(Surface::Window);
    let layers = focus_ring(palette);
    let spreads: Vec<f32> = layers.iter().map(|l| l.spread_radius.into()).collect();
    assert_eq!(spreads, [3.0, 1.0]);
    assert_eq!(layers[0].color, palette.ring());
    assert_eq!(layers[1].color, palette.background());
}
