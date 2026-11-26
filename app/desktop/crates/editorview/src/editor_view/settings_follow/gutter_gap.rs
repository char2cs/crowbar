//! The gap after the line numbers that starts the text where Monaco's does: Monaco gives its
//! numbers at least five digit cells and its folding strip 16px, gpui-kit three cells and 22px.

use gpui::{Font, Pixels, Window, px};

const MONACO_DIGITS: usize = 5;
const MONACO_FOLD_STRIP: Pixels = px(16.);
const KIT_MIN_DIGITS: usize = 3;
const KIT_FOLD_STRIP: Pixels = px(22.);
const DIGITS: &str = "0123456789";

pub(in crate::editor_view) fn monaco_gap(
    lines: usize,
    (font, size): (&Font, Pixels),
    window: &Window,
) -> Pixels {
    let digits = lines.max(1).ilog10() as usize + 1;
    let missing = MONACO_DIGITS.saturating_sub(digits.max(KIT_MIN_DIGITS));
    let text_system = window.text_system();
    let font_id = text_system.resolve_font(font);
    let cell = DIGITS
        .chars()
        .map(|digit| text_system.layout_width(font_id, size, digit))
        .fold(px(0.), Pixels::max);
    (cell * missing + MONACO_FOLD_STRIP - KIT_FOLD_STRIP).max(px(0.))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::Access;
    use crate::editor_view::BODY;
    use crate::editor_view::test_support::opened;

    const MONACO_DIGIT_CELLS: f32 = 5.;

    #[gpui::test]
    fn the_text_starts_after_five_digit_cells_and_the_fold_strip_as_monaco_s_does(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("0\nb", None), Access::Editable, cx);
        window.run_until_parked();
        let first = window.read(|cx| view.read(cx).state.read(cx).range_to_bounds(&(0..1)));
        let (first, body) = (first.unwrap(), window.debug_bounds(BODY).unwrap());
        let digits = first.size.width * MONACO_DIGIT_CELLS;
        let offset = f32::from(first.left() - body.left());
        let expected = f32::from(digits + MONACO_FOLD_STRIP);
        assert!((offset - expected).abs() < 0.01, "{offset} != {expected}");
    }
}
