//! WCAG contrast of two colours.

use gpui::{Hsla, Rgba};

const LINEAR_THRESHOLD: f32 = 0.04045;
const LUMINANCE_WEIGHTS: [f32; 3] = [0.2126, 0.7152, 0.0722];
const FLARE: f32 = 0.05;

fn linear(channel: f32) -> f32 {
    if channel <= LINEAR_THRESHOLD {
        return channel / 12.92;
    }
    ((channel + 0.055) / 1.055).powf(2.4)
}

fn luminance(color: Rgba) -> f32 {
    [color.r, color.g, color.b]
        .into_iter()
        .zip(LUMINANCE_WEIGHTS)
        .map(|(channel, weight)| linear(channel) * weight)
        .sum()
}

fn over(foreground: Rgba, background: Rgba) -> Rgba {
    let mix = |top: f32, bottom: f32| top * foreground.a + bottom * (1.0 - foreground.a);
    Rgba {
        r: mix(foreground.r, background.r),
        g: mix(foreground.g, background.g),
        b: mix(foreground.b, background.b),
        a: 1.0,
    }
}

/// WCAG 2.x contrast ratio (1 to 21) of `foreground` painted over an opaque `background`.
///
/// A translucent foreground is composited over the background first.
pub fn contrast_ratio(foreground: Hsla, background: Hsla) -> f32 {
    let background = Rgba::from(background);
    let foreground = over(Rgba::from(foreground), background);
    let (lighter, darker) = {
        let (a, b) = (luminance(foreground), luminance(background));
        (a.max(b), a.min(b))
    };
    (lighter + FLARE) / (darker + FLARE)
}

#[cfg(test)]
#[path = "contrast_tests.rs"]
mod tests;
