use gpui::{TestAppContext, px};

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn default_size_is_sixteen_pixels(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| Spinner::new("s"));
    assert_eq!((f32::from(bounds.size.width), f32::from(bounds.size.height)), (16.0, 16.0));
    let big = measure(cx, |_, _| Spinner::new("s").size(px(24.0)));
    assert_eq!(f32::from(big.size.width), 24.0);
}
