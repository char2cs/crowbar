//! Colours and shadows of the text field frame.

use gpui::{BoxShadow, Hsla, point, px};

use crate::appearance::Appearance;
use crate::palette::Palette;
use crate::pose::Pose;
use crate::shadow;

const RING_WIDTH: f32 = 3.0;
const RING_ALPHA: f32 = 0.24;
const DARK_FILL: f32 = 0.32;

#[derive(Debug, Clone)]
pub(crate) struct Chrome {
    pub(crate) fill: Hsla,
    pub(crate) border: Hsla,
    pub(crate) shadows: Vec<BoxShadow>,
}

pub(crate) fn chrome(palette: &Palette, appearance: Appearance, focus: Pose) -> Chrome {
    let fill = match appearance {
        Appearance::Light => palette.background(),
        Appearance::Dark => palette.input().opacity(DARK_FILL),
    };
    let mut shadows = shadow::xs(0.05);
    if focus == Pose::Off {
        return Chrome {
            fill,
            border: palette.input(),
            shadows,
        };
    }
    shadows.push(BoxShadow {
        color: palette.ring().opacity(RING_ALPHA),
        offset: point(px(0.0), px(0.0)),
        blur_radius: px(0.0),
        spread_radius: px(RING_WIDTH),
    });
    Chrome {
        fill,
        border: palette.ring(),
        shadows,
    }
}

#[cfg(test)]
#[path = "chrome_tests.rs"]
mod tests;
