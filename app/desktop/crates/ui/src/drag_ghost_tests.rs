use gpui::{AppContext as _, TestAppContext, px};

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn the_ghost_is_a_thirty_two_pixel_pill_of_the_requested_width(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, cx| {
        cx.new(|_| DragGhost::new("Untitled chat", 160.0))
    });
    assert_eq!(bounds.size.width, px(160.0), "requested width");
    assert_eq!(bounds.size.height, px(32.0), "pill height");
}
