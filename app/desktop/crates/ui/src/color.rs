//! Colour construction from the literals of the spec.

use gpui::Rgba;

const BYTE_MAX: f32 = 255.0;

#[allow(
    clippy::cast_possible_truncation,
    clippy::cast_lossless,
    reason = "const context: the value is masked to one byte before the casts"
)]
const fn channel(value: u32, shift: u32) -> f32 {
    (((value >> shift) & 0xff) as u8) as f32 / BYTE_MAX
}

pub(crate) const fn hex(value: u32) -> Rgba {
    Rgba {
        r: channel(value, 24),
        g: channel(value, 16),
        b: channel(value, 8),
        a: channel(value, 0),
    }
}

#[cfg(test)]
#[path = "color_tests.rs"]
mod tests;
