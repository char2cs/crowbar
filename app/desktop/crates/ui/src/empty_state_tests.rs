use gpui::TestAppContext;

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn block_is_at_least_ninety_six_pixels_high(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| EmptyState::new("Nothing here"));
    assert!(f32::from(bounds.size.height) >= 96.0);
}
