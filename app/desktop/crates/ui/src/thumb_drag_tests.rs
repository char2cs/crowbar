use gpui::{ScrollHandle, point, px, size};

use super::*;

#[test]
fn dragging_the_thumb_scrolls_proportionally_and_clamps() {
    let handle = ScrollHandle::new();
    let drag = ThumbDrag::new(
        handle,
        100.0,
        Bounds::new(point(px(0.0), px(0.0)), size(px(6.0), px(20.0))),
    );
    assert!(
        drag.target((10.0, 0.0), 10.0).abs() < f32::EPSILON,
        "no content, nothing to scroll"
    );
}
