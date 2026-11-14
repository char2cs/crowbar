use gpui::TestAppContext;

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn the_wordmark_is_larger_than_the_headline_text(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| Wordmark);
    assert!(f32::from(bounds.size.height) >= SIZE, "{bounds:?}");
    assert!(f32::from(bounds.size.width) > SIZE, "{bounds:?}");
}
