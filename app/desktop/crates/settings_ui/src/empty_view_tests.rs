use super::*;
use crate::test_support::measure;

#[gpui::test]
fn the_empty_text_is_padded_and_visible(cx: &mut gpui::TestAppContext) {
    let bounds = measure(cx, |_, cx| empty_view(cx).into_any_element());
    assert!(f32::from(bounds.size.height) > 16.0);
}
