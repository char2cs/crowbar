use gpui::TestAppContext;

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn pill_is_at_least_sixteen_pixels_high(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| Kbd::new("Ctrl+B"));
    assert!(f32::from(bounds.size.height) >= 16.0);
}
