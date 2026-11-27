use gpui::{Div, Hsla, ParentElement, Styled, div, px};

use crate::panels::chats::reorder::line_box::LineBox;

const HEIGHT: f32 = 2.0;
const CAP: f32 = 4.0;
const CAP_REACH: f32 = -8.0;
const CAP_RISE: f32 = -1.0;

pub(crate) fn drop_line(
    at: LineBox,
    color: Hsla,
) -> Div {
    let cap = div()
        .absolute()
        .left(px(CAP_REACH))
        .top(px(CAP_RISE))
        .size(px(CAP))
        .rounded_full()
        .bg(color);
    div()
        .relative()
        .w(px(at.width))
        .h(px(HEIGHT))
        .rounded_full()
        .bg(color)
        .child(cap)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn the_line_is_two_pixels_thick_and_as_wide_as_asked(cx: &mut gpui::TestAppContext) {
        let at = LineBox {
            left: 0.0,
            top: 0.0,
            width: 120.0,
        };
        let bounds = measure(cx, move |_, _| drop_line(at, gpui::black()));
        assert_eq!(f32::from(bounds.size.height), HEIGHT);
    }
}
