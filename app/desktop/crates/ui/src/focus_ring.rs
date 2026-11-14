//! The 2px focus ring with a 1px background offset.

use gpui::{BoxShadow, point, px};

use crate::palette::Palette;

const OFFSET: f32 = 1.0;
const WIDTH: f32 = 2.0;

/// Shadow stack drawing `ring-2 ring-ring ring-offset-1 ring-offset-background`.
pub(crate) fn focus_ring(palette: &Palette) -> Vec<BoxShadow> {
    let layer = |color, spread| BoxShadow {
        color,
        offset: point(px(0.0), px(0.0)),
        blur_radius: px(0.0),
        spread_radius: px(spread),
    };
    vec![layer(palette.ring(), OFFSET + WIDTH), layer(palette.background(), OFFSET)]
}

#[cfg(test)]
#[path = "focus_ring_tests.rs"]
mod tests;
