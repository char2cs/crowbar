use gpui::IntoElement as _;

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn a_title_alone_is_a_forty_eight_pixel_line_with_twelve_of_padding_and_eight_below(
    cx: &mut gpui::TestAppContext,
) {
    let bounds = measure(cx, |_, cx| {
        title_block("Icons", None, cx).into_any_element()
    });
    assert_eq!(f32::from(bounds.size.height), 68.0);
}
