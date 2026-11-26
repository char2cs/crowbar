//! Letter spacing: Tailwind's `tracking-*` scale, in em of the text it spaces.

use gpui::Pixels;

/// Tailwind `tracking-tight` (-0.025em).
pub const TIGHT: f32 = -0.025;
/// Tailwind `tracking-wider` (0.05em).
pub const WIDER: f32 = 0.05;
/// Tailwind `tracking-widest` (0.1em).
pub const WIDEST: f32 = 0.1;

/// The letter spacing `em` gives text set at `size`, as CSS resolves an em spacing.
#[must_use]
pub fn at(
    em: f32,
    size: Pixels,
) -> Pixels {
    size * em
}

#[cfg(test)]
mod tests {
    use gpui::px;

    use super::{TIGHT, at};

    #[test]
    fn an_em_spacing_scales_with_the_text_size() {
        assert_eq!(at(TIGHT, px(20.0)), px(-0.5));
    }
}
