use gpui::{point, px};

use super::*;

#[test]
fn a_press_becomes_a_drag_after_more_than_eight_pixels() {
    let press = Press {
        index: 0,
        origin: point(px(100.0), px(20.0)),
    };
    assert!(
        !press.has_moved_past_threshold(point(px(105.0), px(26.0))),
        "7.8 pixels"
    );
    assert!(
        !press.has_moved_past_threshold(point(px(108.0), px(20.0))),
        "exactly eight"
    );
    assert!(
        press.has_moved_past_threshold(point(px(106.0), px(27.0))),
        "9.2 pixels"
    );
}
