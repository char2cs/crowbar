use gpui::TestAppContext;

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn five_bars_stack_with_gaps_and_padding(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| skeleton_rows(270.0));
    let expected = 5.0 * 20.0 + 4.0 * 8.0 + 2.0 * 12.0;
    assert_eq!(f32::from(bounds.size.height), expected);
}
