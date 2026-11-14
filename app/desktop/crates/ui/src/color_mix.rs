//! Colour interpolation as CSS transitions do it: in sRGB with premultiplied alpha.

use gpui::{Hsla, Rgba};

/// Blends `from` into `to` at progress `t` (0 to 1).
pub(crate) fn mix(from: Hsla, to: Hsla, t: f32) -> Hsla {
    let (from, to) = (Rgba::from(from), Rgba::from(to));
    let alpha = from.a + (to.a - from.a) * t;
    if alpha <= f32::EPSILON {
        return Hsla::from(Rgba { a: 0.0, ..to });
    }
    let channel = |a: f32, b: f32| (a * from.a + (b * to.a - a * from.a) * t) / alpha;
    Hsla::from(Rgba {
        r: channel(from.r, to.r),
        g: channel(from.g, to.g),
        b: channel(from.b, to.b),
        a: alpha,
    })
}

#[cfg(test)]
#[path = "color_mix_tests.rs"]
mod tests;
