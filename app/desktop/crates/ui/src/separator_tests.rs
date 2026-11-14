use gpui::{ParentElement, Styled, TestAppContext, div, px};

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn lines_are_one_pixel_thick(cx: &mut TestAppContext) {
    let horizontal = measure(cx, |_, _| Separator::horizontal());
    assert_eq!(f32::from(horizontal.size.height), 1.0);
    let vertical = measure(cx, |_, _| gpui::div().h(px(20.0)).child(Separator::vertical()));
    assert_eq!(f32::from(vertical.size.width), 1.0);
}
