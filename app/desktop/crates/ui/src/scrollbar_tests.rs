use gpui::{ParentElement, ScrollHandle, Styled, TestAppContext, div, hsla, px};

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn overlay_does_not_change_layout(cx: &mut TestAppContext) {
    let handle = ScrollHandle::new();
    let bounds = measure(cx, move |_, _| {
        div().relative().size(px(50.0)).child(div().absolute().inset_0().child(Scrollbar::new("bar".into(), handle.clone(), hsla(0.0, 0.0, 0.5, 0.5))))
    });
    assert_eq!(f32::from(bounds.size.width), 50.0);
}
