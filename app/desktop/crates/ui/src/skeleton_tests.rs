use gpui::{TestAppContext, px};

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn block_has_the_requested_size(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| Skeleton::new("sk", px(120.0), px(14.0)));
    assert_eq!((f32::from(bounds.size.width), f32::from(bounds.size.height)), (120.0, 14.0));
}
