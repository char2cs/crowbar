use super::*;
use crate::test_support::measure;

#[gpui::test]
fn a_note_has_the_height_of_one_text_line(cx: &mut gpui::TestAppContext) {
    let bounds = measure(cx, |_, cx| note_text("No hosts.", cx).into_any_element());
    assert!(f32::from(bounds.size.height) > 0.0 && f32::from(bounds.size.height) < 24.0);
}
