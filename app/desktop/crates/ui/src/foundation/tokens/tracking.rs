use gpui::Pixels;

pub const TIGHT: f32 = -0.025;
pub const WIDER: f32 = 0.05;
pub const WIDEST: f32 = 0.1;

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
