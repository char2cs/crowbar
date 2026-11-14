use gpui::TestAppContext;

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn handle_is_twelve_pixels_square(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| DragHandle::new("h"));
    assert_eq!((f32::from(bounds.size.width), f32::from(bounds.size.height)), (12.0, 12.0));
}
