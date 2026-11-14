use gpui::TestAppContext;

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn the_message_fills_the_panel_width_and_keeps_a_minimum_height(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, cx| panel_message("Nothing here", cx));
    assert!(f32::from(bounds.size.height) >= 96.0);
}
