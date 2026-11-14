use gpui::{ParentElement, ScrollHandle, Styled, TestAppContext, div, px};

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn viewport_keeps_its_size_and_content_scrolls(cx: &mut TestAppContext) {
    let handle = ScrollHandle::new();
    let probe = handle.clone();
    let bounds = measure(cx, move |_, _| {
        let tall = div().h(px(600.0)).w(px(100.0));
        div().h(px(100.0)).w(px(100.0)).child(ScrollArea::new("sa", &probe, tall))
    });
    assert_eq!((f32::from(bounds.size.width), f32::from(bounds.size.height)), (100.0, 100.0));
    assert!(f32::from(handle.max_offset().height) > 400.0, "content is taller than the viewport");
}
