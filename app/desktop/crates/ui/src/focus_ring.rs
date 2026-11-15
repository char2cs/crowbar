//! The 2px focus ring with a 1px background offset.

use gpui::{BoxShadow, point, px};

use crate::palette::Palette;

const OFFSET: f32 = 1.0;
const WIDTH: f32 = 2.0;

pub(crate) fn focus_ring(palette: &Palette) -> Vec<BoxShadow> {
    let layer = |color, spread| BoxShadow {
        color,
        offset: point(px(0.0), px(0.0)),
        blur_radius: px(0.0),
        spread_radius: px(spread),
    };
    vec![
        layer(palette.ring(), OFFSET + WIDTH),
        layer(palette.background(), OFFSET),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::surface::Surface;
    use crate::theme::Theme;

    #[test]
    fn ring_is_two_pixels_beyond_a_one_pixel_offset() {
        let palette = Theme::default().palette(Surface::Window);
        let layers = focus_ring(palette);
        let spreads: Vec<f32> = layers.iter().map(|l| l.spread_radius.into()).collect();
        assert_eq!(spreads, [3.0, 1.0]);
        assert_eq!(layers[0].color, palette.ring());
        assert_eq!(layers[1].color, palette.background());
    }
}
