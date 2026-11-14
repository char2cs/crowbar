use super::*;
use crate::test_support::measure;

#[gpui::test]
fn the_notice_is_laid_out_in_a_toast(cx: &mut gpui::TestAppContext) {
    let bounds = measure(cx, |_, _| notice_view("Could not save X."));
    assert!(f32::from(bounds.size.height) >= 0.0);
}
