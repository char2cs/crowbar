//! Colours and shadows of the text field frame.

use gpui::{BoxShadow, Hsla, point, px};

use crate::foundation::Appearance;
use crate::foundation::Palette;
use crate::foundation::Pose;
use crate::foundation::tokens::opacity;
use crate::foundation::tokens::shadow;

const RING_WIDTH: f32 = 3.0;
const RING_ALPHA: f32 = 0.24;

#[derive(Debug, Clone)]
pub(crate) struct Chrome {
    pub(crate) fill: Hsla,
    pub(crate) border: Hsla,
    pub(crate) shadows: Vec<BoxShadow>,
}

pub(crate) fn chrome(
    palette: &Palette,
    appearance: Appearance,
    focus: Pose,
) -> Chrome {
    let fill = match appearance {
        Appearance::Light => palette.background(),
        Appearance::Dark => palette.input().opacity(opacity::DARK_INPUT),
    };
    if focus == Pose::Off {
        return resting(palette, fill);
    }
    focused(palette, fill)
}

fn resting(
    palette: &Palette,
    fill: Hsla,
) -> Chrome {
    Chrome {
        fill,
        border: palette.input(),
        shadows: shadow::xs(opacity::PERCENT_5),
    }
}

fn focused(
    palette: &Palette,
    fill: Hsla,
) -> Chrome {
    let mut shadows = shadow::xs(opacity::PERCENT_5);
    shadows.push(BoxShadow {
        color: palette.ring().opacity(RING_ALPHA),
        offset: point(px(0.0), px(0.0)),
        blur_radius: px(0.0),
        spread_radius: px(RING_WIDTH),
        inset: false,
    });
    Chrome {
        fill,
        border: palette.ring(),
        shadows,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::foundation::Surface;
    use crate::foundation::Theme;

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
}
